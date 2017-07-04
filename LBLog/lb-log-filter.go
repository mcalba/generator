package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

func main() {
	f, err := os.Open("lb.log")
	if err != nil {
		log.Println(err)
		return
	}

	defer f.Close()

	var reqlist []string
	var reslist []string
	var tearlist []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()

		if strings.Contains(line, "OnDescribeSemiSetupRequest") == false &&
			strings.Contains(line, "OnOnDemandSessionRequest") == false &&
			strings.Contains(line, "OnSessionModifyNotification") == false &&
			strings.Contains(line, "OnTeardownNotification") == false {
			continue
		}

		strs := strings.Split(line, ",")
		strs = strings.Split(strs[7], ":")

		if strings.Contains(line, "OnDescribeSemiSetupRequest") || strings.Contains(line, "OnOnDemandSessionRequest") {
			reqlist = append(reqlist, strings.TrimSpace(strs[1]))
		} else if strings.Contains(line, "OnSessionModifyNotification") {
			if strings.Contains(line, "VirtualVODConnectionThread") {
				strs = strings.Split(strs[1], "[")
				strs = strings.Split(strs[1], "]")
				reslist = append(reslist, strs[0])
			} else {
				reslist = append(reslist, strings.TrimSpace(strs[1]))
			}
		} else if strings.Contains(line, "OnTeardownNotification") {
			if strings.Contains(line, "VirtualVODConnectionThread") {
				strs = strings.Split(strs[1], "[")
				strs = strings.Split(strs[1], "]")
				tearlist = append(tearlist, strs[0])
			} else {
				tearlist = append(tearlist, strings.TrimSpace(strs[1]))
			}
		}
	}

	sort.Strings(reqlist)
	sort.Strings(reslist)
	sort.Strings(tearlist)

	var errlist []string

	num := 0
	i := 0
	j := 0
	k := 0
	for {
		if i >= len(reqlist) {
			break
		}

		find := false
		for {

			if j >= len(reslist) {
				break
			}

			if reqlist[i] == reslist[j] {
				find = true
				j++
				break
			} else if reqlist[i] < reslist[j] {
				break
			} else {
				j++
			}
		}

		if find == false {

			for {
				if k >= len(tearlist) {
					break
				}

				if reqlist[i] == tearlist[k] {
					num++
					find = true
					k++
					break
				} else if reqlist[i] < tearlist[k] {
					break
				} else {
					k++
				}
			}

			if find == false {
				errlist = append(errlist, reqlist[i])
			}
		}
		i++
	}

	fmt.Println("Max Bandwidth :", num)
	num = 0

	event, err := os.Open("event.log")
	if err != nil {
		log.Println(err)
		return
	}

	defer event.Close()

	ss := bufio.NewScanner(event)
	for ss.Scan() {
		line := ss.Text()

		if strings.Contains(line, "Selected") {
			strs := strings.Split(line, ",")
			strs = strings.Split(strs[3], " ")

			find := false
			for i := 0; i < len(errlist); i++ {
				if errlist[i] == strs[7] {
					find = true
					break
				}
			}
			if find == true {
				fmt.Println("Not Response Session. Server IP:", strs[1], "StreamID:", strs[7])
				num++
			}
		}
	}

	fmt.Println("Not Response Number :", num)

}
