package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		log.Println("Usage: vod-log-filter [vod event log file name]")
		return
	}

	FileName := os.Args[1]
	f, err := os.Open(FileName)
	if err != nil {
		log.Println(err)
		return
	}

	defer f.Close()

	reqlist := make(map[string]int)

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()

		if strings.Contains(line, "Reserved Session") == false &&
			strings.Contains(line, "0x10001") == false {
			continue
		}

		strs := strings.Split(line, ",")
		time, _ := strconv.Atoi(strs[2])

		if strings.Contains(line, "Reserved Session") {

			strs = strings.Split(strs[4], " ")
			strs = strings.Split(strs[2], "(")
			strs = strings.Split(strs[1], ")")
			streamID := strs[0]

			reqlist[streamID] = time
		} else if strings.Contains(line, "0x10001") {

			strs = strings.Split(strs[11], "[")
			strs = strings.Split(strs[1], "]")
			streamID := strs[0]

			value, ok := reqlist[streamID]
			if ok {
				if time-value > 1 {
					fmt.Println("response time :", time-value, "streamID :", streamID)
				}
				delete(reqlist, streamID)
			}
		}
	}
}
