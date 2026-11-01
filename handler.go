package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

var (
	monthReplacer = strings.NewReplacer(
		" jan ", " Jan ",
		" feb ", " Feb ",
		" mar ", " Mar ",
		" apr ", " Apr ",
		" maj ", " May ",
		" jun ", " Jun ",
		" jul ", " Jul ",
		" aug ", " Aug ",
		" sep ", " Sep ",
		" okt ", " Oct ",
		" nov ", " Nov ",
		" dec ", " Dec ",
	)
)

func handle(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()
	log.Println("Client connected:", conn.RemoteAddr())

	reader := bufio.NewReader(conn)
	combinedBuffer := bytes.Buffer{}

	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			log.Println("Client disconnected:", conn.RemoteAddr())
			break
		}
		if err != nil {
			println("Failed to read from buffer:", err.Error())
			return
		}
		err = combinedBuffer.WriteByte(b)
		if err != nil {
			println("Failed to write to combined buffer:", err.Error())
			return
		}
	}
	message := combinedBuffer.Bytes()
	mq.Publish(mqttTopic+"/raw", 0, false, []byte(formatHex(message)))

	parseMessage(message)
}

func formatHex(data []byte) string {
	out := ""
	for _, b := range data {
		out += fmt.Sprintf("%02x", b)
	}
	return out
}
