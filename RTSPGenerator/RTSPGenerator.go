package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/beatgammit/rtsp"
)

type configInfo struct {
	fileName    string
	destIP      string
	serviceCode string
	contentType string
}

type gslbSetup struct {
	address        string
	ServiceCode    string `json:"serviceCode"`
	ClientIP       string `json:"clientIp"`
	ProtocolType   string `json:"protocolType"`
	ContentType    string `json:"contentType"`
	Content        string `json:"content"`
	RequestBitrate string `json:"requestBitrate"`
	StreamingType  string `json:"streamingType"`
	Path           string `json:"path"`
}

type gslbresponse struct {
	ResultCode  int      `json:"resultCode"`
	OneTimeURL  []string `json:"oneTimeUrl"`
	ErrorString string   `json:"errorString"`
}

func gslbsetup(info *gslbSetup) (string, error) {
	doc, _ := json.Marshal(info)
	buff := bytes.NewBuffer(doc)
	url := "http://" + info.address + "/command/demandOtu"
	resp, err := http.Post(url, "application/json", buff)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		res, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		data := gslbresponse{}
		json.Unmarshal(res, &data)
		if data.ResultCode != 200 {
			return "", fmt.Errorf(data.ErrorString)
		}
		return data.OneTimeURL[0], err

	default:
		return "", fmt.Errorf("Status Code = %d", resp.StatusCode)
	}
}

// RTSPSetup "RTSP Setup Function"
func RTSPSetup(url string, localIP string, seq int) (*rtsp.Session, *rtsp.Response, error) {
	client := rtsp.NewSession()
	client.LocalIP = localIP

	start := time.Now()
	res, err := client.Describe(url, "goClient")
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
	res, err = client.Setup(url, transport, "goClient")
	if err != nil {
		return nil, nil, err
	}

	if res.StatusCode != 200 && res.StatusCode != 301 {
		return nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] glb setup response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	if res.StatusCode == 301 {

		strurl := res.Header.Get("Location")
		start = time.Now()
		res, err = client.VODSetup(strurl, transport, "goClient")
		if err != nil {
			return nil, nil, err
		}

		if res.StatusCode != 200 {
			return nil, nil, fmt.Errorf("RTSP Receved %v", res.Status)
		}
		log.Printf("[%d] vod setup response time: %d ms, url = %v", seq, (int(time.Now().Sub(start)) / 1000000), strurl)
	}

	return client, res, err
}

// RTSPPlay "RTSP Play Fuction"
func RTSPPlay(c *rtsp.Session, url string, id string, t int, seq int) error {
	start := time.Now()
	res, err := c.Play(url, id, "goClient")
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("RTSP Receved %v", res.Status)
	}
	log.Printf("[%d] play response time: %d ms", seq, (int(time.Now().Sub(start)) / 1000000))

	during := time.Now()
	heartbeat := time.Now()

	for {

		buf := make([]byte, 64*1024)
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
		buf := make([]byte, 64*1024)
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

	FileName := flag.String("filename", "", "generation info file name. mandatory ")
	Address := flag.String("addr", "", "glb server addresss. mandatory (ex) 127.0.0.1:1554")
	SessionCount := flag.Int("count", 0, "the number of session. default is generation info file count")
	Interval := flag.Int("interval", 1000, "session generation interval (millisecond)")
	PlayTime := flag.Int("playtime", 900, "play time (second)")
	PlayInterval := flag.Int("playinterval", 0, "time to play after setup (second)")
	StreamingType := flag.String("type", "static", "streaming type. adaptive or static")
	UseGSLB := flag.Bool("gslb", true, "use gslb. true or false (ex) -gslb=false")

	flag.Parse()

	if *FileName == "" || *Address == "" {
		log.Println("RTSPGenerator v1.0.5")
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
			if len(data) < 4 {
				log.Println("invalid config data : ", token[i])
				i++
				continue
			}

			cfg := configInfo{}

			cfg.fileName = data[0]
			cfg.destIP = data[1]
			cfg.serviceCode = data[2]
			cfg.contentType = data[3]

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

			var glburl string
			if *UseGSLB {
				info := gslbSetup{}

				info.address = *Address
				info.ServiceCode = cfglist[num].serviceCode
				info.ClientIP = cfglist[num].destIP
				info.ProtocolType = "rtsp"
				info.ContentType = cfglist[num].contentType
				info.RequestBitrate = "H"
				info.StreamingType = *StreamingType

				if strings.Contains(cfglist[num].fileName, "/") {
					info.Path = string(cfglist[num].fileName[0:(strings.LastIndex(cfglist[num].fileName, "/"))])
					info.Content = string(cfglist[num].fileName[(strings.LastIndex(cfglist[num].fileName, "/"))+1 : len(cfglist[num].fileName)])
				} else {
					info.Content = cfglist[num].fileName
				}

				start := time.Now()
				glburl, err = gslbsetup(&info)
				if err != nil {
					log.Printf("[%d] error: %s", n, err)
					return
				}

				log.Printf("[%d] gslb response time: %d ms", n, (int(time.Now().Sub(start)) / 1000000))
			} else {
				glburl = "rtsp://" + *Address + "/" + cfglist[num].fileName
			}

			client, res, err := RTSPSetup(glburl, cfglist[num].destIP, n)
			if err != nil {
				log.Printf("[%d] error: url : %s", n, err)
				return
			}

			if *PlayInterval > 0 {
				time.Sleep(time.Duration(*PlayInterval * 1000000000))
			}

			err = RTSPPlay(client, glburl, res.Header.Get("Session"), t, n)
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
}
