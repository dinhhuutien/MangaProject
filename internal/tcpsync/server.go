package tcpsync

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"sync"

	"mangahub/pkg/models"
)

// Server nhận progress events và broadcast cho mọi TCP client
type Server struct {
	addr string

	mu      sync.Mutex
	clients map[net.Conn]struct{}

	broadcast <-chan models.ProgressUpdate
}

func New(addr string, broadcast <-chan models.ProgressUpdate) *Server {
	return &Server{
		addr:      addr,
		clients:   make(map[net.Conn]struct{}),
		broadcast: broadcast,
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	log.Printf("TCP Sync listening on %s", s.addr)

	// Goroutine: nhận event từ channel và broadcast
	go s.broadcastLoop()

	// Accept loop
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("tcp accept:", err)
			continue
		}
		s.addClient(conn)
		log.Printf("TCP client connected: %s", conn.RemoteAddr().String())

		// (optional) đọc để phát hiện disconnect
		go s.readLoop(conn)
	}
}

func (s *Server) addClient(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[conn] = struct{}{}
}

func (s *Server) removeClient(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, conn)
	_ = conn.Close()
}

func (s *Server) readLoop(conn net.Conn) {
	// Client không cần gửi gì; read để biết khi nào client disconnect
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		// ignore any incoming line
	}
	s.removeClient(conn)
	log.Printf("TCP client disconnected: %s", conn.RemoteAddr().String())
}

func (s *Server) broadcastLoop() {
	for evt := range s.broadcast {
		b, err := json.Marshal(evt)
		if err != nil {
			log.Println("tcp marshal:", err)
			continue
		}
		// newline-delimited JSON để client đọc theo dòng (TCP là stream)
		b = append(b, '\n')

		s.mu.Lock()
		for conn := range s.clients {
			if _, err := conn.Write(b); err != nil {
				// lỗi write => remove client
				delete(s.clients, conn)
				_ = conn.Close()
			}
		}
		s.mu.Unlock()
	}
}
