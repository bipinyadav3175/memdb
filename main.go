package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	storeFlag := flag.String("store", "", "backend to use: map, syncmap, or sharded")
	flag.Parse()

	if *storeFlag != "" {
		kind, err := ParseStoreKind(*storeFlag)
		if err != nil {
			fmt.Println(err)
			return
		}
		if err := os.Setenv("MEMORY_DB_STORE", string(kind)); err != nil {
			fmt.Println("Unable to set store backend env", "err:", err)
			return
		}
	}

	store = NewStore()

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
