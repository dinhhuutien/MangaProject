package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	server := "127.0.0.1:7070"
	if len(os.Args) > 1 {
		server = os.Args[1]
	}

	serverAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		panic(err)
	}

	// Bind local port random (:0) để vừa send SUBSCRIBE vừa receive noti trên cùng socket
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Subscribe
	if _, err := conn.WriteToUDP([]byte("SUBSCRIBE"), serverAddr); err != nil {
		panic(err)
	}

	fmt.Println("UDP monitor subscribed to:", server)
	fmt.Println("Local addr:", conn.LocalAddr().String())
	fmt.Println("Waiting for notifications...")

	buf := make([]byte, 4096)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("read error:", err)
			continue
		}
		fmt.Printf("FROM %s: %s\n", from.String(), string(buf[:n]))
	}
}
