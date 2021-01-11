package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beatgammit/rtsp"
)

type configInfo struct {
	fileName string
	destIP   string
	ifName   string
}

// RTSPSetup "RTSP Setup Function"
func RTSPSetup(url string, localIP string, IfName string, seq int) (*rtsp.Session, *rtsp.Response, net.Conn, net.Conn, string, error) {
	client := rtsp.NewSession()
	client.LocalIP = localIP
	client.IfName = IfName

	start := time.Now()
	res, err := client.Describe(url)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	if res.StatusCode != 200 {
		return nil, nil, nil, nil, "", fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] describe response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	resposeSDP, err := rtsp.ParseSdp(&io.LimitedReader{R: res.Body, N: res.ContentLength})
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	vodAddress := strings.Split(resposeSDP.ConnectionInformation, " ")

	var transport = "CIP/TCP; unicast; destination=" + localIP

	strurl := "rtsp://" + vodAddress[2] + ":554" + "/" + strings.Split(url, "/")[3]
	start = time.Now()
	res, err = client.VODSetup(strurl, transport)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	if res.StatusCode != 200 {
		return nil, nil, nil, nil, "", fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] vod setup response time: %d ms, url = %v", seq, (int(time.Now().Sub(start)) / 1000000), strurl)

	// 데이터 소켓 연결하기
	localAddr, err := net.ResolveIPAddr("ip", localIP)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	LocalBindAddr := &net.TCPAddr{
		IP:   localAddr.IP,
		Zone: IfName,
	}

	dailer := net.Dialer{
		LocalAddr: LocalBindAddr,
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	conn, err := dailer.Dial("tcp", vodAddress[2]+":32127")
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	var sessionid int
	if sessionid, err = strconv.Atoi(strings.Split(res.Header.Get("Session"), ";")[0]); err != nil {
		return nil, nil, nil, nil, "", err
	}

	data := make([]byte, 12)
	binary.BigEndian.PutUint32(data, 10002)
	binary.BigEndian.PutUint32(data[4:8], 4)
	binary.BigEndian.PutUint32(data[8:12], uint32(sessionid))

	_, err = io.WriteString(conn, string(data))
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	buf := make([]byte, 4)
	_, err = conn.Read(buf)

	if err != nil && err != io.EOF {
		return nil, nil, nil, nil, "", err
	}

	if err == io.EOF {
		return nil, nil, nil, nil, "", err
	}

	dailerCICP := net.Dialer{
		LocalAddr: LocalBindAddr,
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	connCICP, err := dailerCICP.Dial("tcp", vodAddress[2]+":32127")
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	client.GetCICPForSDK(connCICP, strings.Split(res.Header.Get("Session"), ";")[0], 10000)

	return client, res, conn, connCICP, strurl, err
}

// RTSPPlay "RTSP Play Fuction"
func RTSPPlay(c *rtsp.Session, conn net.Conn, connCICP net.Conn, url string, id string, t int, seq int) error {
	//start := time.Now()
	res, err := c.Play(url)
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("RTSP Receved %v", res.Status)
	}
	//log.Printf("[%d] play response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	during := time.Now()
	heartbeat := time.Now()

	for {

		buf := make([]byte, 64*1024)
		_, err := conn.Read(buf)

		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF {
			log.Printf("EOF")
			break
		}

		if time.Duration(t*1000000000) <= time.Now().Sub(during) {
			res, err = c.Teardown(url)
			if err != nil {
				return err
			}

			buf := make([]byte, 10*1024)
			_, err := res.Body.Read(buf)

			if err != nil && err != io.EOF {
				return err
			}

			break
		}

		if time.Duration(5*1000000000) <= time.Now().Sub(heartbeat) {
			res, err = c.GetCICPForSDK(connCICP, id, 200)
			if err != nil {
				return err
			}

			if err != nil && err != io.EOF {
				return err
			}

			heartbeat = time.Now()
		}
	}

	for {
		buf := make([]byte, 64*1024)
		_, err := res.Body.Read(buf)

		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF {
			break
		}
	}

	res.Body.Close()
	connCICP.Close()
	conn.Close()

	return err

}

func main() {

	FileName := flag.String("filename", "", "generation info file name. mandatory ")
	Address := flag.String("addr", "", "glb server addresss. mandatory (ex) 127.0.0.1:1554 or [fe80::a236:9fff:fe27:eb02]:1554")
	SessionCount := flag.Int("count", 0, "the number of session. default is generation info file count")
	Interval := flag.Int("interval", 1000, "session generation interval (millisecond)")
	PlayTime := flag.Int("playtime", 900, "play time (second)")
	PlayInterval := flag.Int("playinterval", 0, "time to play after setup (second)")

	flag.Parse()

	if *FileName == "" || *Address == "" {
		log.Println("SDKGeneratorForCICP v1.0.0")
		flag.Usage()
		return
	}

	configData, err := ioutil.ReadFile(*FileName)
	if err != nil {
		log.Println("config file read file: ", err)
		return
	}

	cfData := string(configData)

	token := strings.Split(cfData, "\n")

	var cfglist []configInfo

	i := 0
	for i < len(token) {
		if token[i] != "" {
			data := strings.Fields(token[i])
			if len(data) != 3 {
				log.Println("invalid config data : ", token[i])
				i++
				continue
			}

			cfg := configInfo{}

			cfg.fileName = data[0]
			cfg.destIP = data[1]
			cfg.ifName = data[2]

			cfglist = append(cfglist, cfg)
		}
		i++
	}

	if len(cfglist) == 0 {
		log.Println("cfglist is zero.")
		//return
	}

	if *SessionCount == 0 {
		*SessionCount = len(cfglist)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	wg := new(sync.WaitGroup)

	for i := 0; i < *SessionCount; i++ {
		num := i

		if num >= len(cfglist) {
			num %= len(cfglist)
		}

		wg.Add(1)
		go func(t int, n int) {

			defer wg.Done()

			url := "rtsp://" + *Address + "/" + cfglist[num].fileName

			client, res, conn, connCICP, newUrl, err := RTSPSetup(url, cfglist[num].destIP, cfglist[num].ifName, n)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}

			if *PlayInterval > 0 {
				time.Sleep(time.Duration(*PlayInterval * 1000000000))
			}

			err = RTSPPlay(client, conn, connCICP, newUrl, strings.Split(res.Header.Get("Session"), ";")[0], t, n)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}

			log.Printf("[%d] Session End", n)
		}(*PlayTime, i)

		if *Interval > 1 {
			time.Sleep(time.Duration(*Interval * 1000000))
		}
	}
	wg.Wait()
	log.Println("the all end")

	return
}
