package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kz26/m3u8"
)

type configInfo struct {
	protocol    string
	fileName    string
	clientID    string
	destIP      string
	destPort    int
	speed       int
	serviceCode string
	contentType string
	bitrateType string
}

type gslbSetup struct {
	serverIP       string
	serverPort     string
	ServiceCode    string `json:"serviceCode"`
	ClientIP       string `json:"clientIp"`
	ProtocolType   string `json:"protocolType"`
	ContentType    string `json:"contentType"`
	Content        string `json:"content"`
	RequestBitrate string `json:"requestBitrate"`
	StreamingType  string `json:"streamingType"`
}

type gslbresponse struct {
	ResultCode  int      `json:"resultCode"`
	OneTimeURL  []string `json:"oneTimeUrl"`
	ErrorString string   `json:"errorString"`
}

func gslbsetup(info *gslbSetup) (string, error) {
	doc, _ := json.Marshal(info)
	buff := bytes.NewBuffer(doc)
	url := "http://" + info.serverIP + ":" + info.serverPort + "/command/demandOtu"
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

func glbSetup(u *url.URL, c *http.Client) (*url.URL, error) {

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "dahakan")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 301 {
		return nil, fmt.Errorf("Received HTTP %v for %v", resp.StatusCode, u.String())
	}

	uri, err := u.Parse(resp.Header.Get("Location"))
	if err != nil {
		return nil, err
	}

	return uri, err

}

func getContent(u *url.URL, c *http.Client) (io.ReadCloser, *url.URL, error) {

	//log.Println(u.String())
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("User-Agent", "dahakan")
	resp, err := c.Do(req)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("Received HTTP %v for %v", resp.StatusCode, u.String())
	}

	resurl := resp.Request

	return resp.Body, resurl.URL, err

}

func absolutize(rawurl string, u *url.URL) (uri *url.URL, err error) {
	suburl := rawurl
	uri, err = u.Parse(suburl)
	if err != nil {
		return
	}

	if rawurl == u.String() {
		return
	}

	if !uri.IsAbs() { // relative URI
		if rawurl[0] == '/' { // from the root
			suburl = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, rawurl)
		} else { // last element
			splitted := strings.Split(u.String(), "/")
			splitted[len(splitted)-1] = rawurl

			suburl = strings.Join(splitted, "/")
		}
	}

	suburl, err = url.QueryUnescape(suburl)
	if err != nil {
		return
	}

	uri, err = u.Parse(suburl)
	if err != nil {
		return
	}

	return
}

func download(u *url.URL, c *http.Client, f float64) {
	start := time.Now()
	content, _, err := getContent(u, c)
	if err != nil {
		log.Println("error:", err)
		return
	}

	for {
		buf := make([]byte, 32*1024)
		_, err := content.Read(buf)

		if err != nil && err != io.EOF {
			log.Println("error:", err)
			break
		}

		if err == io.EOF {
			break
		}
	}
	content.Close()

	restime := int(f*1000) - (int(time.Now().Sub(start)) / 1000000)
	time.Sleep(time.Duration(restime * 1000000))
}

func getPlaylist(u *url.URL, t int, c *http.Client) {

	for t > 0 {

		content, _, err := getContent(u, c)
		if err != nil {
			log.Println("error:", err)
			break
		}

		playlist, listType, err := m3u8.DecodeFrom(content, true)
		if err != nil {
			log.Println("error:", err)
			break
		}
		content.Close()

		if listType != m3u8.MEDIA && listType != m3u8.MASTER {
			log.Println("error: Not a valid playlist")
			break
		}

		if listType == m3u8.MEDIA {

			mediapl := playlist.(*m3u8.MediaPlaylist)

			for idx, segment := range mediapl.Segments {
				if segment == nil {
					chunk := mediapl.Segments[idx-1]
					if chunk != nil {
						msURL, err := absolutize(chunk.URI, u)
						if err != nil {
							log.Println("error:", err)
							break
						}
						download(msURL, c, chunk.Duration)
						t -= int(chunk.Duration)
						break
					}
				}
			}
		} else {
			log.Println("error: invaild m3u8 Type")
			break
		}
	}
}

func main() {

	if len(os.Args) < 6 {
		log.Println("HLSGenerator v1.0.1")
		log.Println("Usage: HLSGenerator [generation_info_file_name] [server_ip] [the_number_of_session] [session_generation_interval] [play_time] [ server_port ] ")
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
			if len(data) != 9 {
				log.Println("invalid config data : ", token[i])
				i++
				continue
			}
			cfg := configInfo{}

			cfg.protocol = data[0]
			cfg.fileName = data[1]
			cfg.clientID = data[2]
			cfg.destIP = data[3]
			cfg.destPort, err = strconv.Atoi(data[4])
			cfg.speed, err = strconv.Atoi(data[5])
			cfg.serviceCode = data[6]
			cfg.contentType = data[7]
			cfg.bitrateType = data[8]

			cfglist = append(cfglist, cfg)
		}
		i++
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	wg := new(sync.WaitGroup)

	for i := 0; i < SessionCount; i++ {
		info := gslbSetup{}
		num := i

		if num >= len(cfglist) {
			num %= len(cfglist)
		}

		info.serverIP = ServerIP
		info.serverPort = Port
		info.ServiceCode = cfglist[num].serviceCode
		info.ClientIP = cfglist[num].destIP
		info.ProtocolType = cfglist[num].protocol
		info.ContentType = cfglist[num].contentType
		info.Content = cfglist[num].fileName
		info.RequestBitrate = cfglist[num].bitrateType
		info.StreamingType = "static"

		localAddr, err := net.ResolveIPAddr("ip", info.ClientIP)
		if err != nil {
			log.Println("error:", err)
			continue
		}

		LocalBindAddr := &net.TCPAddr{IP: localAddr.IP}

		start := time.Now()
		otuurl, err := gslbsetup(&info)
		if err != nil {
			log.Println("error:", err)
			continue
		}
		log.Println("gslb response time:", (int(time.Now().Sub(start)) / 1000000), "ms")

		theURL, err := url.Parse(otuurl)
		if err != nil {
			log.Println("error:", err)
			continue
		}
		wg.Add(1)
		go func(u *url.URL, t int, n int) {

			defer wg.Done()

			var httpTransport = &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				Dial: (&net.Dialer{
					LocalAddr: LocalBindAddr,
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).Dial,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			}

			client := &http.Client{
				Transport: httpTransport,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			start := time.Now()
			url, err := glbSetup(u, client)
			if err != nil {
				log.Println("error:", err)
				return
			}
			log.Println("glb response time:", (int(time.Now().Sub(start)) / 1000000), "ms")

			start = time.Now()
			content, url, err := getContent(url, client)
			if err != nil {
				log.Println("error:", err)
				return
			}
			log.Println("vod response time:", (int(time.Now().Sub(start)) / 1000000), "ms")

			playlist, listType, err := m3u8.DecodeFrom(content, true)
			if err != nil {
				log.Println("error:", err)
				return
			}
			content.Close()

			if listType != m3u8.MEDIA && listType != m3u8.MASTER {
				log.Println("error: Not a valid playlist")
				return
			}

			if listType == m3u8.MASTER {

				// HLS Live ( OTM Channel )

				masterpl := playlist.(*m3u8.MasterPlaylist)
				for _, variant := range masterpl.Variants {

					if variant != nil {

						msURL, err := absolutize(variant.URI, url)
						if err != nil {
							log.Println("error:", err)
							return
						}
						getPlaylist(msURL, t, client)
						break
					}
				}
			} else if listType == m3u8.MEDIA {

				// HLS VOD ( SKYLIFE Prime Movie Pack )

				mediapl := playlist.(*m3u8.MediaPlaylist)

				for _, segment := range mediapl.Segments {
					if segment != nil {
						msURL, err := absolutize(segment.URI, u)
						if err != nil {
							log.Println("error:", err)
							break
						}
						download(msURL, client, segment.Duration)
						t -= int(segment.Duration)
					} else {
						break
					}

					if t <= 0 {
						break
					}
				}
			}
			log.Printf("[%d] Session End", n)
		}(theURL, PlayTime, i)

		time.Sleep(time.Duration(Interval * 1000000000))
	}
	wg.Wait()
	log.Println("the all end")
}
