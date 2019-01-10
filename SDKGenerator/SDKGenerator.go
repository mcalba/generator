package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/beatgammit/rtsp"
)

type configInfo struct {
	fileName string
	destIP   string
}

// GetADVSchedules "GET ADList Function"
func GetADVSchedules(id string, addr string, localIP string) error {
	localAddr, err := net.ResolveIPAddr("ip", localIP)
	if err != nil {
		return err
	}
	LocalBindAddr := &net.TCPAddr{IP: localAddr.IP}
	transport := &http.Transport{
		Dial: (&net.Dialer{
			LocalAddr: LocalBindAddr,
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
	}
	client := &http.Client{
		Transport: transport,
	}

	url := "http://" + addr + "/adm/adv-schedules/" + id + "?format=cic"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("ADM Receved %v", resp.Status)
	}

	for {
		buf := make([]byte, 32*1024)
		_, err := resp.Body.Read(buf)

		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF {
			break
		}
	}
	resp.Body.Close()
	transport.CloseIdleConnections()

	return nil
}

// RTSPSetup "RTSP Setup Function"
func RTSPSetup(url string, localIP string, seq int) (*rtsp.Session, *rtsp.Response, net.Conn, error) {
	client := rtsp.NewSession()
	client.LocalIP = localIP

	start := time.Now()
	res, err := client.Describe(url, "Castanets RTSP/1.1")
	if err != nil {
		return nil, nil, nil, err
	}

	if res.StatusCode != 200 {
		return nil, nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] describe response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	_, err = rtsp.ParseSdp(&io.LimitedReader{R: res.Body, N: res.ContentLength})
	if err != nil {
		return nil, nil, nil, err
	}

	var transport = "CIP/CIP/TCP; unicast"

	start = time.Now()
	res, err = client.Setup(url, transport, "Castanets RTSP/1.1")
	if err != nil {
		return nil, nil, nil, err
	}

	if res.StatusCode != 200 && res.StatusCode != 301 {
		return nil, nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] glb setup response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	strurl := res.Header.Get("Location")
	start = time.Now()
	res, err = client.VODSetup(strurl, transport, "Castanets RTSP/1.1")
	if err != nil {
		return nil, nil, nil, err
	}

	if res.StatusCode != 200 {
		return nil, nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] vod setup response time: %d ms, url = %v", seq, (int(time.Now().Sub(start)) / 1000000), strurl)

	//광고 리스트 얻어오기

	err = GetADVSchedules(strings.Split(res.Header.Get("Session"), ";")[0], strings.Split(strurl, "/")[2], localIP)
	if err != nil {
		return nil, nil, nil, err
	}

	// 데이터 소켓 연결하기
	var tcpAddress string
	token := strings.Split(res.Header.Get("Transport"), ";")
	for _, port := range token {

		if strings.Contains(port, "server_port") {
			tcpAddress = strings.Split(strings.Split(strurl, "/")[2], ":")[0] + ":" + strings.Split(port, "=")[1]
		}
	}

	if tcpAddress == "" {
		return nil, nil, nil, fmt.Errorf("Not Exist Server Port")
	}

	localAddr, err := net.ResolveIPAddr("ip", localIP)
	if err != nil {
		return nil, nil, nil, err
	}

	LocalBindAddr := &net.TCPAddr{IP: localAddr.IP}

	dailer := net.Dialer{
		LocalAddr: LocalBindAddr,
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	conn, err := dailer.Dial("tcp", tcpAddress)
	if err != nil {
		return nil, nil, nil, err
	}

	_, err = client.SetParameterForSDK(strurl, "goclient")
	if err != nil {
		return nil, nil, nil, err
	}

	return client, res, conn, err
}

// RTSPPlay "RTSP Play Fuction"
func RTSPPlay(c *rtsp.Session, conn net.Conn, url string, id string, t int, seq int) error {
	//start := time.Now()
	res, err := c.Play(url, id, "Castanets RTSP/1.1")
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
			res, err = c.GetParameterForSDK(url, id)
			if err != nil {
				return err
			}

			buf := make([]byte, 10*1024)
			_, err := res.Body.Read(buf)

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
	conn.Close()

	return err

}

func main() {

	FileName := flag.String("filename", "", "generation info file name. mandatory ")
	Address := flag.String("addr", "", "glb server addresss. mandatory (ex) 127.0.0.1:1554")
	SessionCount := flag.Int("count", 0, "the number of session. default is generation info file count")
	Interval := flag.Int("interval", 1000, "session generation interval (millisecond)")
	PlayTime := flag.Int("playtime", 900, "play time (second)")
	PlayInterval := flag.Int("playinterval", 0, "time to play after setup (second)")

	flag.Parse()

	if *FileName == "" || *Address == "" {
		log.Println("SDKGenerator v1.0.1")
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
			if len(data) != 2 {
				log.Println("invalid config data : ", token[i])
				i++
				continue
			}

			cfg := configInfo{}

			cfg.fileName = data[0]
			cfg.destIP = data[1]

			cfglist = append(cfglist, cfg)
		}
		i++
	}

	if len(cfglist) == 0 {
		log.Println("cfglist is zero.")
		return
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

			client, res, conn, err := RTSPSetup(url, cfglist[num].destIP, n)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}

			if *PlayInterval > 0 {
				time.Sleep(time.Duration(*PlayInterval * 1000000000))
			}

			err = RTSPPlay(client, conn, url, strings.Split(res.Header.Get("Session"), ";")[0], t, n)
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
