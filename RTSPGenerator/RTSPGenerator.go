package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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

// RTSPSetup "RTSP Setup Function"
func RTSPSetup(url string, localIP string, seq int) (*rtsp.Session, *rtsp.Response, error) {
	client := rtsp.NewSession()
	client.LocalIP = localIP

	start := time.Now()
	res, err := client.Describe(url)
	if err != nil {
		return nil, nil, err
	}

	if res.StatusCode != 200 {
		return nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] describe response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	_, err = rtsp.ParseSdp(&io.LimitedReader{R: res.Body, N: res.ContentLength})
	if err != nil {
		return nil, nil, err
	}

	var transport = "RTP/AVP/TCP; unicast; interleaved=0-1"

	start = time.Now()
	res, err = client.Setup(url, transport)
	if err != nil {
		return nil, nil, err
	}

	if res.StatusCode != 200 && res.StatusCode != 301 {
		return nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] glb setup response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	if res.StatusCode == 301 {

		start = time.Now()
		res, err = client.VODSetup(res.Header.Get("Location"), transport)
		if err != nil {
			return nil, nil, err
		}

		if res.StatusCode != 200 {
			return nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
		}
		log.Printf("[%d] vod setup response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))
	}

	return client, res, err
}

// RTSPPlay "RTSP Play Fuction"
func RTSPPlay(c *rtsp.Session, url string, id string, t int) error {
	res, err := c.Play(url, id)
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("RTSP Receved %v", res.Status)
	}

	during := time.Now()
	heartbeat := time.Now()

	for {

		buf := make([]byte, 32*1024)
		_, err := res.Body.Read(buf)

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

			defer res.Body.Close()
			break
		}

		if time.Duration(14*1000000000) <= time.Now().Sub(heartbeat) {
			res, err = c.GetParameter(url, id)
			if err != nil {
				return err
			}

			heartbeat = time.Now()
		}
	}

	for {
		buf := make([]byte, 32*1024)
		_, err := res.Body.Read(buf)

		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF {
			break
		}
	}

	return err

}

func main() {

	FileName := flag.String("filename", "", "generation info file name. ")
	Address := flag.String("addr", "", "glb server addresss. (ex) 127.0.0.1:1554")
	SessionCount := flag.Int("count", 0, "the number of session. default is generation info file count")
	Interval := flag.Int("interval", 1000, "session generation interval (millisecond)")
	PlayTime := flag.Int("playtime", 900, "play time (second)")

	flag.Parse()

	if *FileName == "" || *Address == "" {
		log.Println("RTSPGenerator v1.0.3")
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

			client, res, err := RTSPSetup(url, cfglist[num].destIP, n)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}

			err = RTSPPlay(client, url, res.Header.Get("Session"), t)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}

			log.Printf("[%d] Session End", n)
		}(*PlayTime, i)

		time.Sleep(time.Duration(*Interval * 1000000))
	}
	wg.Wait()
	log.Println("the all end")
}
