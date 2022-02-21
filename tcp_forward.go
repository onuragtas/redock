package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
)

const (
	CONN_HOST = "0.0.0.0"
	CONN_PORT = "10000"
	CONN_TYPE = "tcp"
)

var projects = []string{
	"localhost:10001",
	"localhost:10002",
	"localhost:10003",
	"localhost:10004",
	"localhost:10005",
	"localhost:10006",
	"localhost:10007",
	"localhost:10008",
	"localhost:10009",
	"localhost:10010",
}

func TcpForward() {
	go func() {
		for {
			l, err := net.Listen(CONN_TYPE, CONN_HOST+":"+CONN_PORT)
			if err != nil {
				fmt.Println("Error listening:", err.Error())
				os.Exit(1)
			}
			// Close the listener when the application closes.
			defer l.Close()
			fmt.Println("Listening on " + CONN_HOST + ":" + CONN_PORT)
			for {
				// Listen for an incoming connection.
				conn, err := l.Accept()
				if err != nil {
					fmt.Println("Error accepting: ", err.Error())
					os.Exit(1)
				}
				// Handle connections in a new goroutine.
				handleRequest(conn)
				//conn.Close()
			}

		}
	}()
}

func handleRequest(conn net.Conn) {

	handled := false
	for _, v := range projects {
		local, err := net.Dial("tcp", v)
		if err == nil {
			go func() { io.Copy(conn, local) }()
			go func() { io.Copy(local, conn) }()
			handled = true
		}
	}

	if !handled {
		conn.Close()
	}
}

func Write(conn net.Conn, content string) (int, error) {
	writer := bufio.NewWriter(conn)
	number, err := writer.WriteString(content)
	if err == nil {
		err = writer.Flush()
	}
	return number, err
}
