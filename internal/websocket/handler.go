package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"mangahub/pkg/models"
)

// upgrade http cho websocket
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type client struct {
	hub  *ChatHub
	conn *websocket.Conn
	send chan []byte
}

func HandleWebSocket(hub *ChatHub) gin.HandlerFunc {
	return func(c *gin.Context) {
		// upgrade connection
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Println("websocket upgrade:", err)
			return
		}

		// Lấy username từ Query params
		username := c.Query("username")
		if username == "" {
			username = "anonymous"
		}

		// Tạo client
		sendChan := make(chan []byte, 256) // tạo channel
		client := &client{
			hub:  hub,
			conn: conn,
			send: sendChan,
		}
		// Đăng ký client vào hub
		hub.mu.Lock()
		hub.clients[conn] = username
		hub.sendChans[conn] = sendChan
		hub.mu.Unlock()

		// Chạy goroutine để gửi và nhận messages
		go client.readPump()
		go client.writePump()
	}
}

// Tạo readPump (nhận dữ liệu từ client)
func (c *client) readPump() {
	defer func() {
		c.hub.unregister <- c.conn
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		// Đọc message từ websocet
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		// parse json
		var msg models.ChatMessage
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("error unmarshaling message: %v", err)
			continue
		}

		// set mốc thời gian
		if msg.Timestamp == 0 {
			msg.Timestamp = time.Now().Unix()
		}

		// broadcast message
		c.hub.broadcast <- msg
	}
}

// Tạo writePump (gửi dữ liệu đến client)
func (c *client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:

			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if !ok {
				// đóng channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// gửi message đến websocket
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// đóng writer
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
