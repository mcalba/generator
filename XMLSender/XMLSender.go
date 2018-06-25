package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strings"
)

func main() {

	FileName := flag.String("filename", "", "xml file path. mandatory")
	Address := flag.String("addr", "", "server addresss. mandatory (ex) 127.0.0.1:18085")

	flag.Parse()

	if *FileName == "" || *Address == "" {
		log.Println("XMLSender v1.0.0")
		flag.Usage()
		return
	}

	// connect to this socket
	conn, err := net.Dial("tcp", *Address)
	if err != nil {
		log.Println(err)
		return
	}

	defer conn.Close()

	xmlData, err := ioutil.ReadFile(*FileName)
	if err != nil {
		log.Println("xml file read file: ", err)
		return
	}

	xml := string(xmlData)

	conn.Write(xmlData)

	// ADSAdapter 의 eADS XML 은 ETX 가 없으면 parsing 이 되지 않음
	if strings.Contains(xml, "eADS") {
		conn.Write([]byte{0x03})
	}

	buff, err := ioutil.ReadAll(conn)
	if err != nil {
		log.Println("recv fail: ", err)
		return
	}
	fmt.Println(string(buff))
}
