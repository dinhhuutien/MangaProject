package udpnotify

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type Notification struct {
	Type      string `json:"type"` // "notification"
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

type Server struct {
	addr string

	mu      sync.Mutex
	clients map[string]*net.UDPAddr // key = ip:port

	conn *net.UDPConn
}

func New(addr string) *Server {
	return &Server{
		addr:    addr,
		clients: make(map[string]*net.UDPAddr),
	}
}

func (s *Server) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	s.conn = conn

	log.Printf("UDP Notify listening on %s", s.addr)

	buf := make([]byte, 2048)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("udp read:", err)
			continue
		}

		msg := strings.TrimSpace(string(buf[:n]))
		upper := strings.ToUpper(msg)

		// Protocol đơn giản:
		// - client gửi "SUBSCRIBE" -> server lưu addr để broadcast
		// - client gửi "UNSUBSCRIBE" -> remove
		if upper == "SUBSCRIBE" {
			s.mu.Lock()
			s.clients[clientAddr.String()] = clientAddr
			s.mu.Unlock()
			log.Printf("UDP subscribed: %s (total=%d)", clientAddr.String(), s.count())
			continue
		}

		if upper == "UNSUBSCRIBE" {
			s.mu.Lock()
			delete(s.clients, clientAddr.String())
			s.mu.Unlock()
			log.Printf("UDP unsubscribed: %s (total=%d)", clientAddr.String(), s.count())
			continue
		}

		// ignore other messages
	}
}

func (s *Server) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

func (s *Server) Broadcast(message string) {
	if s.conn == nil {
		log.Println("udp conn not started yet")
		return
	}

	noti := Notification{
		Type:      "notification",
		Message:   message,
		Timestamp: time.Now().Unix(),
	}
	b, err := json.Marshal(noti)
	if err != nil {
		log.Println("udp marshal:", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, addr := range s.clients {
		if _, err := s.conn.WriteToUDP(b, addr); err != nil {
			log.Printf("udp send to %s failed: %v", key, err)
		}
	}
}
