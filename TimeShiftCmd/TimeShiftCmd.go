package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type streamInfo struct {
	FileName        string   `json:"filename"`
	Bandwidth       int      `json:"bandwidth"`
	SourceURL       string   `json:"sourceUrl"`
	M3U8name        string   `json:"m3u8name"`
	Includem3u8list []string `json:"includem3u8List"`
	ScheduleID      string   `json:"scheduleId"`
	DistType        string   `json:"distType"`
	VideoEncrypt    int      `json:"videoEncrypt"`
	AudioEncrypt    int      `json:"audioEncrypt"`
}

type addChannelInfo struct {
	ServiceID       string       `json:"serviceId"`
	Seqtime         int          `json:"seqtime"`
	Maxmanifestnum  int          `json:"maxmanifestnum"`
	StreamInfo      []streamInfo `json:"streamInfo"`
	SourceInterface string       `json:"sourceInterface"`
	SaveFilePath    string       `json:"saveFilepath"`
}

type addchannelResponse struct {
	ResultCode  int          `json:"resultCode"`
	StreamInfo  []streamInfo `json:"streamInfo"`
	ErrorString string       `json:"errorString"`
}

type modifyChannelInfo struct {
	ServiceID      string `json:"serviceId"`
	Maxmanifestnum int    `json:"maxmanifestnum"`
}

type modifychannelResponse struct {
	ResultCode  int    `json:"resultCode"`
	ErrorString string `json:"errorString"`
}

type deleteChannelInfo struct {
	ScheduleID []string `json:"scheduleId"`
}

type deleteInfo struct {
	ResultCode  int    `json:"resultCode"`
	ScheduleID  string `json:"scheduleId"`
	ErrorString string `json:"errorString"`
}

type deletechannelResponse struct {
	DeleteInfo []deleteInfo `json:"deleteInfo"`
}

type clientList struct {
	IP     string `json:"ip"`
	Status string `json:"status"`
}

type channel struct {
	BackupURL      string       `json:"backupUrl"`
	Bandwidth      int          `json:"bandwidth"`
	ClientList     []clientList `json:"clientList"`
	DistType       string       `json:"disttype"`
	FileName       string       `json:"filename"`
	ScheduleID     string       `json:"scheduleId"`
	ScheduleStatus string       `json:"scheduleStatus"`
	ServiceID      string       `json:"serviceId"`
	SourceURL      string       `json:"sourceUrl"`
}

type channelstatusResponse struct {
	ResultCode  int       `json:"resultCode"`
	Channel     []channel `json:"channel"`
	TotalCount  int       `json:"totalCount"`
	ErrorString string    `json:"errorString"`
}

func addChannel(info *addChannelInfo, addr string) error {
	url := "http://" + addr + "/command/channel/add"
	doc, _ := json.Marshal(info)
	buff := bytes.NewBuffer(doc)
	resp, err := http.Post(url, "application/json", buff)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		res, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		data := addchannelResponse{}
		json.Unmarshal(res, &data)
		if data.ResultCode != 200 {
			return fmt.Errorf("%s. service id : %s", data.ErrorString, info.ServiceID)
		}

		log.Printf("add success. service id : %s", info.ServiceID)

		return err

	default:
		return fmt.Errorf("Status Code = %d, %s. service id : %s", resp.StatusCode, resp.Status, info.ServiceID)
	}
}

func deleteChannel(info *deleteChannelInfo, id string, addr string) error {
	url := "http://" + addr + "/command/channel/delete"
	doc, _ := json.Marshal(info)
	buff := bytes.NewBuffer(doc)
	resp, err := http.Post(url, "application/json", buff)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		res, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		data := deletechannelResponse{}
		json.Unmarshal(res, &data)
		for _, delete := range data.DeleteInfo {
			if delete.ResultCode != 200 {
				log.Printf("%s. serivce id : %s", delete.ErrorString, id)
			} else {
				log.Printf("delete success. service id : %s", id)
			}
		}
		return err

	default:
		return fmt.Errorf("Status Code = %d, %s. service id : %s", resp.StatusCode, resp.Status, id)
	}
}

func modifyChannel(info *modifyChannelInfo, addr string) error {
	url := "http://" + addr + "/command/channel/modify"
	doc, _ := json.Marshal(info)
	buff := bytes.NewBuffer(doc)
	resp, err := http.Post(url, "application/json", buff)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		res, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		data := modifychannelResponse{}
		json.Unmarshal(res, &data)
		if data.ResultCode != 200 {
			return fmt.Errorf("%s. service id : %s", data.ErrorString, info.ServiceID)
		}
		log.Printf("modify success. service id : %s, max chunk number : %d", info.ServiceID, info.Maxmanifestnum)
		return err

	default:
		return fmt.Errorf("Status Code = %d, %s. service id : %s", resp.StatusCode, resp.Status, info.ServiceID)
	}
}

func channelStatus(addr string) error {
	url := "http://" + addr + "/command/channel/status/all"
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		res, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		data := channelstatusResponse{}
		json.Unmarshal(res, &data)
		if data.ResultCode != 200 {
			return fmt.Errorf(data.ErrorString)
		}

		var channelList []channel
		var i int
		for _, info := range data.Channel {
			if info.DistType == "timeshift" {
				i++
				channelList = append(channelList, info)
			}
		}

		if i > 0 {
			fmt.Printf("\n")
			fmt.Println("total count :", i)
			for _, info := range channelList {
				fmt.Println("----------------------------------------------")
				fmt.Println("filename	serviceId	bandwidth")
				fmt.Printf("%s		%s		%d\n\n", info.FileName, info.ServiceID, info.Bandwidth)
				fmt.Println("server ip			status")
				for _, status := range info.ClientList {
					fmt.Printf("%s			%s\n", status.IP, status.Status)
				}
			}
			fmt.Println("----------------------------------------------")
			fmt.Printf("\n")
		} else {
			log.Println("Not Exist Timeshift Channel")
		}

		return err

	default:
		return fmt.Errorf("Status Code = %d, %s", resp.StatusCode, resp.Status)
	}
}

func getScheduleID(id string, addr string) (string, error) {
	url := "http://" + addr + "/command/channel/status/all"
	resp, err := http.Get(url)
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

		data := channelstatusResponse{}
		json.Unmarshal(res, &data)
		if data.ResultCode != 200 {
			return "", fmt.Errorf(data.ErrorString)
		}

		for _, info := range data.Channel {
			if id == info.ServiceID {
				return info.ScheduleID, err
			}
		}

		return "", fmt.Errorf("Not Exist Service id : %s", id)

	default:
		return "", fmt.Errorf("Status Code = %d, %s", resp.StatusCode, resp.Status)
	}
}

func main() {

	FileName := flag.String("filename", "", "config file path. required when add")
	Address := flag.String("addr", "", "server addresss. mandatory (ex) 127.0.0.1:18085")
	Command := flag.String("command", "", "request command. mandatory. add, modify, delete, status")
	SeqTime := flag.Int("seqtime", 5, "chunk length (sec)")
	MaxManifestNum := flag.Int("maxnumber", 720, "chunk save number")
	VideoEncrypt := flag.Int("videoencrypt", 1, "use video encryption. 1 or 0 ")
	AudioEncrypt := flag.Int("audioencrypt", 0, "use audio encryption. 1 or 0  (default 0)")
	ServiceID := flag.String("serviceid", "", "service id. required for modify and delete.")
	ModifyNum := flag.Int("modifynumber", 0, "modify chunk save number. required when modify.")

	flag.Parse()

	if *Command == "" || *Address == "" {
		fmt.Println("TimeShiftCmd v1.0.0")
		flag.Usage()
		return
	}

	if *Command == "add" && *FileName == "" {
		fmt.Println("Not Exist filename.")
		flag.Usage()
		return
	}

	if *Command == "modify" && *ServiceID == "" {
		fmt.Println("Not Exist ServiceID.")
		flag.Usage()
		return
	}

	if *Command == "modify" && *ModifyNum < 1 {
		fmt.Println("Invalid ModifyNum.")
		flag.Usage()
		return
	}

	if *Command == "delete" && *ServiceID == "" {
		fmt.Println("Not Exist ServiceID.")
		flag.Usage()
		return
	}

	switch *Command {
	case "add":
		configData, err := ioutil.ReadFile(*FileName)
		if err != nil {
			log.Println("json file read file: ", err)
			return
		}
		cfData := string(configData)
		token := strings.Split(cfData, "\n")

		for _, cfg := range token {
			if cfg != "" {
				data := strings.Fields(cfg)
				if len(data) != 6 {
					log.Println("invalid config data : ", cfg)
					continue
				}

				info := streamInfo{}
				info.FileName = data[0]
				info.Bandwidth, err = strconv.Atoi(data[1])
				if err != nil {
					log.Println("invalid config data : ", cfg)
					continue
				}
				info.SourceURL = data[2]
				info.DistType = "timeshift"
				info.VideoEncrypt = *VideoEncrypt
				info.AudioEncrypt = *AudioEncrypt

				addInfo := addChannelInfo{}

				addInfo.ServiceID = data[3]
				addInfo.Seqtime = *SeqTime
				addInfo.Maxmanifestnum = *MaxManifestNum
				addInfo.SourceInterface = data[4]
				addInfo.SaveFilePath = data[5]
				addInfo.StreamInfo = append(addInfo.StreamInfo, info)

				err = addChannel(&addInfo, *Address)
				if err != nil {
					log.Println(err)
					continue
				}
			}
		}
	case "delete":
		scheduleID, err := getScheduleID(*ServiceID, *Address)
		if err != nil {
			log.Println(err)
			return
		}

		deleteInfo := deleteChannelInfo{}
		deleteInfo.ScheduleID = append(deleteInfo.ScheduleID, scheduleID)

		err = deleteChannel(&deleteInfo, *ServiceID, *Address)
		if err != nil {
			log.Println(err)
			return
		}
	case "status":
		err := channelStatus(*Address)
		if err != nil {
			log.Println(err)
			return
		}
	case "modify":
		modifyInfo := modifyChannelInfo{}
		modifyInfo.ServiceID = *ServiceID
		modifyInfo.Maxmanifestnum = *ModifyNum

		err := modifyChannel(&modifyInfo, *Address)
		if err != nil {
			log.Println(err)
			return
		}
	default:
		fmt.Println("Not Support Command.")
		flag.Usage()
	}
}
