package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/grafov/m3u8"
)

type configInfo struct {
	fileName    string
	destIP      string
	serviceCode string
	contentType string
	bitrateType string
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

func glbSetup(u *url.URL, c *http.Client) (*url.URL, error) {

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Castis-User-Agent", "dahakan")
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

func vodsetup(u *url.URL, c *http.Client) (io.ReadCloser, *url.URL, error) {

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("X-Castis-User-Agent", "dahakan")
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

func getContent(u *url.URL, c *http.Client) (io.ReadCloser, *url.URL, error) {

	log.Println(u.String())
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

	log.Printf("Received HTTP %v for %v\n", resp.StatusCode, u.String())

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

func download(u *url.URL, c *http.Client, f float64) error {
	start := time.Now()
	content, _, err := getContent(u, c)
	if err != nil {
		return err
	}

	defer content.Close()

	for {
		buf := make([]byte, 32*1024)
		_, err := content.Read(buf)

		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF {
			break
		}
	}

	log.Printf("Data Received Complete %v\n", u.String())

	restime := int(f*1000) - (int(time.Now().Sub(start)) / 1000000)
	time.Sleep(time.Duration(restime * 1000000))

	return nil
}

func disconnectDownload(u *url.URL, c *http.Client, f float64) error {
	start := time.Now()
	content, _, err := getContent(u, c)
	if err != nil {
		return err
	}

	for {
		buf := make([]byte, 32*1024)
		_, err := content.Read(buf)

		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF {
			content.Close()
			break
		}
	}

	restime := int(f*1000) - (int(time.Now().Sub(start)) / 1000000)
	time.Sleep(time.Duration(restime * 1000000))

	return nil
}

func getPlaylist(v *m3u8.Variant, u *url.URL, t int, c *http.Client, n int) {
	msURL, err := absolutize(v.URI, u)
	if err != nil {
		log.Println("error:", err)
		return
	}

	content, _, err := getContent(msURL, c)
	if err != nil {
		log.Println("error:", err)
		return
	}

	playlist, listType, err := m3u8.DecodeFrom(content, true)
	if err != nil {
		log.Println("error:", err)
		return
	}
	content.Close()

	if listType != m3u8.MEDIA {
		log.Println("error: invaild m3u8 Type")
		return
	}

	mediapl := playlist.(*m3u8.MediaPlaylist)
	if mediapl.Closed == false {
		// live ( OTM Channel )
		log.Printf("[%d] Adaptive Channel Session (OTM Channel)", n)
		for t > 0 {
			// chunk download
			for idx, segment := range mediapl.Segments {
				if segment == nil {
					chunk := mediapl.Segments[idx-1]
					if chunk != nil {
						chunkURL, err := absolutize(chunk.URI, u)
						if err != nil {
							log.Println("error:", err)
							return
						}
						err = download(chunkURL, c, chunk.Duration)
						if err != nil {
							log.Println("error:", err)
							return
						}
						t -= int(chunk.Duration)
					}
				}
				break
			}
			//m3u8 update
			{
				content, _, err = getContent(msURL, c)
				if err != nil {
					log.Println("error:", err)
					return
				}

				playlist, listType, err = m3u8.DecodeFrom(content, true)
				if err != nil {
					log.Println("error:", err)
					return
				}
				content.Close()

				mediapl = playlist.(*m3u8.MediaPlaylist)
			}
		}
	} else {
		// vod ( OTM VOD )
		log.Printf("[%d] Adaptive VOD Session (OTM VOD)", n)
		var subplaylist m3u8.Playlist
		var audioplaylist m3u8.Playlist
		for _, alt := range v.Alternatives {
			// sub, audio list download
			if alt.URI != "" {
				if alt.Type == "SUBTITLES" && subplaylist == nil {
					altURL, err := absolutize(alt.URI, u)
					if err != nil {
						log.Println("error:", err)
						return
					}

					content, _, err := getContent(altURL, c)
					if err != nil {
						log.Println("error:", err)
						return
					}

					subplaylist, listType, err = m3u8.DecodeFrom(content, true)
					if err != nil {
						log.Println("error:", err)
						return
					}
					content.Close()

					if listType != m3u8.MEDIA {
						log.Println("error: invaild m3u8 Type")
						return
					}
				}

				if alt.Type == "AUDIO" && audioplaylist == nil {
					altURL, err := absolutize(alt.URI, u)
					if err != nil {
						log.Println("error:", err)
						return
					}

					content, _, err := getContent(altURL, c)
					if err != nil {
						log.Println("error:", err)
						return
					}

					audioplaylist, listType, err = m3u8.DecodeFrom(content, true)
					if err != nil {
						log.Println("error:", err)
						return
					}
					content.Close()

					if listType != m3u8.MEDIA {
						log.Println("error: invaild m3u8 Type")
						return
					}
				}
			}
		}

		var subpl *m3u8.MediaPlaylist
		var audiopl *m3u8.MediaPlaylist

		if subplaylist != nil {
			subpl = subplaylist.(*m3u8.MediaPlaylist)
		}
		if audioplaylist != nil {
			audiopl = audioplaylist.(*m3u8.MediaPlaylist)
		}

		for idx, segment := range mediapl.Segments {
			start := time.Now()
			if subpl != nil {
				// sub download
				subseg := subpl.Segments[idx]
				if subseg != nil {
					msURL, err := absolutize(subseg.URI, u)
					if err != nil {
						log.Println("error:", err)
						return
					}
					err = download(msURL, c, 0)
					if err != nil {
						log.Println("error:", err)
						return
					}
				}
			}

			if audiopl != nil {
				// audio download
				audioseg := audiopl.Segments[idx]
				if audioseg != nil {
					msURL, err := absolutize(audioseg.URI, u)
					if err != nil {
						log.Println("error:", err)
						return
					}
					err = download(msURL, c, 0)
					if err != nil {
						log.Println("error:", err)
						return
					}
				}
			}

			// chunk download
			if segment != nil {
				msURL, err := absolutize(segment.URI, u)
				if err != nil {
					log.Println("error:", err)
					return
				}
				err = download(msURL, c, segment.Duration)
				if err != nil {
					log.Println("error:", err)
					return
				}
			} else {
				return
			}

			t -= int(int(time.Now().Sub(start)) / 1000000000)
			if t <= 0 {
				break
			}
		}
	}
}

func main() {

	FileName := flag.String("filename", "", "generation info file path. mandatory")
	Address := flag.String("addr", "", "server addresss. mandatory (ex) 127.0.0.1:18085")
	SessionCount := flag.Int("count", 0, "the number of session. default is generation info file count")
	Interval := flag.Int("interval", 1000, "session generation interval (millisecond)")
	PlayTime := flag.Int("playtime", 900, "play time (second)")
	StreamingType := flag.String("type", "static", "streaming type. adaptive or static")
	UseGSLB := flag.Bool("gslb", true, "use gslb. true or false")
	DisableKeepAlive := flag.Bool("disablekeepalive", false, "disable keepalive. true or false")

	flag.Parse()

	if *FileName == "" || *Address == "" {
		log.Println("HLSGenerator v1.0.8")
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
			if len(data) != 5 {
				log.Println("invalid config data : ", token[i])
				i++
				continue
			}
			cfg := configInfo{}

			cfg.fileName = data[0]
			cfg.destIP = data[1]
			cfg.serviceCode = data[2]
			cfg.contentType = data[3]
			cfg.bitrateType = data[4]

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
		wg.Add(1)
		go func(t int, n int) {
			defer wg.Done()
			num := n

			if num >= len(cfglist) {
				num %= len(cfglist)
			}

			localAddr, err := net.ResolveIPAddr("ip", cfglist[num].destIP)
			if err != nil {
				log.Printf("[%d] error: %s", i, err)
				return
			}

			LocalBindAddr := &net.TCPAddr{IP: localAddr.IP}

			var glburl string
			if *UseGSLB {
				info := gslbSetup{}

				info.address = *Address
				info.ServiceCode = cfglist[num].serviceCode
				info.ClientIP = cfglist[num].destIP
				info.ProtocolType = "http"
				info.ContentType = cfglist[num].contentType
				info.RequestBitrate = cfglist[num].bitrateType
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
				glburl = "http://" + *Address + "/" + cfglist[num].serviceCode + "/" + cfglist[num].fileName + "?AdaptiveType=HLS"
			}

			theURL, err := url.Parse(glburl)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}

			var httpTransport = &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				Dial: (&net.Dialer{
					LocalAddr: LocalBindAddr,
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).Dial,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				//Android
				DisableKeepAlives: *DisableKeepAlive,
			}

			client := &http.Client{
				Transport: httpTransport,
				Timeout:   5 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			start := time.Now()
			url, err := glbSetup(theURL, client)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}
			log.Printf("[%d] glb response time: %d ms", n, (int(time.Now().Sub(start)) / 1000000))

			start = time.Now()
			content, url, err := vodsetup(url, client)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}
			log.Printf("[%d] vod response time: %d ms", n, (int(time.Now().Sub(start)) / 1000000))

			playlist, listType, err := m3u8.DecodeFrom(content, true)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}
			content.Close()

			if listType != m3u8.MEDIA && listType != m3u8.MASTER {
				log.Printf("[%d] error: Not a valid playlist", n)
				return
			}

			if listType == m3u8.MASTER {
				// HLS Adaptive
				masterpl := playlist.(*m3u8.MasterPlaylist)
				for _, variant := range masterpl.Variants {
					if variant != nil {
						getPlaylist(variant, url, t, client, n)
						break
					}
				}
			} else if listType == m3u8.MEDIA {
				mediapl := playlist.(*m3u8.MediaPlaylist)
				if mediapl.Closed == false {
					// HLS Live ( OTM Channel ). Static
					log.Printf("[%d] Static Channel Session (OTM Channel)", n)
					for t > 0 {
						content, _, err := getContent(url, client)
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
								if idx > 0 {
									if segment == nil {
										chunk := mediapl.Segments[idx-1]
										if chunk != nil {
											msURL, err := absolutize(chunk.URI, url)
											if err != nil {
												log.Println("error:", err)
												break
											}
											err = download(msURL, client, chunk.Duration)
											if err != nil {
												log.Println("error:", err)
												break
											}
											t -= int(chunk.Duration)
											break
										}
									}
								}
							}
						} else {
							log.Println("error: invaild m3u8 Type")
							break
						}
					}
				} else {
					// HLS VOD ( SKYLIFE Prime Movie Pack )
					log.Printf("[%d] Static VOD Session (Skylife Prime Movie Pack)", n)
					for _, segment := range mediapl.Segments {
						if segment != nil {
							msURL, err := absolutize(segment.URI, url)
							if err != nil {
								log.Printf("[%d] error: %s", n, err)
								break
							}
							err = download(msURL, client, segment.Duration)
							if err != nil {
								log.Println("error:", err)
								break
							}
							t -= int(segment.Duration)
						} else {
							break
						}

						if t <= 0 {
							break
						}
					}

				}

			}
			log.Printf("[%d] Session End", n)
		}(*PlayTime, i)

		time.Sleep(time.Duration(*Interval * 1000000))
	}
	wg.Wait()
	log.Println("the all end")
}
