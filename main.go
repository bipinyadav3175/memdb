package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
)

func main() {
	listener, err := net.Listen("tcp", ":5555")
	if err != nil {
		fmt.Println("Unable to start listener", "err:", err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Unable to accept tcp connection", "err:", err)
			continue
		}

		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
		value, err := parseRESP(reader)
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Println("Unable to parse RESP data", "err:", err)
			return
		}

		args, ok := value.([]interface{})
		if !ok {
			_, _ = conn.Write([]byte("-ERR invalid command\r\n"))
			continue
		}

		resp := handleRESPCommand(toStrings(args))
		if _, err := conn.Write([]byte(resp)); err != nil {
			return
		}
	}
}
