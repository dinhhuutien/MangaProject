package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	addr := "127.0.0.1:9090"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("Connected to TCP sync:", addr)
	fmt.Println("Waiting for progress updates...")

	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		fmt.Println(sc.Text())
	}
	fmt.Println("Disconnected.")
}
