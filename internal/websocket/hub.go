package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"

	"mangahub/pkg/models"
)

// Kết nối client
type ClientConnection struct {
	Conn     *websocket.Conn
	Username string
}

// Hub duy trì kết nối client
type ChatHub struct {
	mu         sync.Mutex
	clients    map[*websocket.Conn]string
	sendChans  map[*websocket.Conn]chan []byte
	broadcast  chan models.ChatMessage
	register   chan ClientConnection
	unregister chan *websocket.Conn
}

// Tạo mới hub
func NewHub() *ChatHub {
	return &ChatHub{
		clients:    make(map[*websocket.Conn]string),
		sendChans:  make(map[*websocket.Conn]chan []byte),
		broadcast:  make(chan models.ChatMessage),
		register:   make(chan ClientConnection),
		unregister: make(chan *websocket.Conn),
	}
}

// Chạy loop để handle connection vs broadcasting
func (h *ChatHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.Conn] = client.Username
			h.mu.Unlock()
			log.Printf("Client %s connected", client.Username)
		case conn := <-h.unregister:
			h.mu.Lock()
			if username, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				if sendChan, ok := h.sendChans[conn]; ok {
					close(sendChan)
					delete(h.sendChans, conn)
				}
				conn.Close()
				log.Printf("Client %s disconnected", username)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			data, err := json.Marshal(message)
			if err != nil {
				log.Println("Error marshalling message:", err)
				continue
			}

			h.mu.Lock()
			for conn, sendChan := range h.sendChans {
				select {
				case sendChan <- data:

				default:
					if username, ok := h.clients[conn]; ok {
						log.Printf("Client %s send channel full, removing", username)
					}
					delete(h.clients, conn)
					delete(h.sendChans, conn)
					close(sendChan)
					conn.Close()
				}
			}
			h.mu.Unlock()
		}
	}
}
