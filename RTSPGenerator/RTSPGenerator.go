package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
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
}

// RTSPSetup "RTSP Setup Function"
func RTSPSetup(url string, localIP string) (*rtsp.Session, *rtsp.Response, error) {
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
	log.Println("describe response time:", (int(time.Now().Sub(start)) / 1000000), "ms")

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
	log.Println("glb setup response time:", (int(time.Now().Sub(start)) / 1000000), "ms")

	if res.StatusCode == 301 {
		// client = rtsp.NewSession()
		// client.LocalIP = localIP

		start = time.Now()
		res, err = client.VODSetup(res.Header.Get("Location"), transport)
		if err != nil {
			return nil, nil, err
		}

		if res.StatusCode != 200 {
			return nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
		}
		log.Println("vod setup response time:", (int(time.Now().Sub(start)) / 1000000), "ms")
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
			return err
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

	if len(os.Args) < 6 {
		log.Println("RTSPGenerator v1.0.3")
		log.Println("Usage: RTSPGenerator [generation_info_file_name] [server_ip] [the_number_of_session] [session_generation_interval] [play_time] [ server_port ] ")
		return
	}

	FileName := os.Args[1]
	ServerIP := os.Args[2]
	SessionCount, err := strconv.Atoi(os.Args[3])
	if err != nil {
		log.Println("Not Valid Session Count")
		return
	}
	Interval, err := strconv.Atoi(os.Args[4])
	if err != nil {
		log.Println("Not Valid Interval Time")
		return
	}
	PlayTime, err := strconv.Atoi(os.Args[5])
	if err != nil {
		log.Println("Not Valid Play Time")
		return
	}
	Port := os.Args[6]

	configData, err := ioutil.ReadFile(FileName)
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

	runtime.GOMAXPROCS(runtime.NumCPU())

	wg := new(sync.WaitGroup)

	for i := 0; i < SessionCount; i++ {
		num := i

		if num >= len(cfglist) {
			num %= len(cfglist)
		}

		wg.Add(1)
		go func(t int, n int) {

			defer wg.Done()

			url := "rtsp://" + ServerIP + ":" + Port + "/" + cfglist[num].fileName

			client, res, err := RTSPSetup(url, cfglist[num].destIP)
			if err != nil {
				log.Println("error:", err)
				return
			}

			err = RTSPPlay(client, url, res.Header.Get("Session"), t)
			if err != nil {
				log.Println("error:", err)
				return
			}

			log.Printf("[%d] Session End", n)
		}(PlayTime, i)

		time.Sleep(time.Duration(Interval * 1000000000))
	}
	wg.Wait()
	log.Println("the all end")
}
