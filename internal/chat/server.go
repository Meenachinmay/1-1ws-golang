package chat

import (
	"1-1ws/internal/websocket"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type Server struct {
	users      map[string]*User
	broadcast  chan *Message
	register   chan *User
	unregister chan *User
	mu         sync.Mutex
}

func NewChatServer() *Server {
	return &Server{
		users:      make(map[string]*User),
		broadcast:  make(chan *Message),
		register:   make(chan *User),
		unregister: make(chan *User),
	}
}

func (s *Server) Run() {
	go func() {
		for {
			select {
			case user := <-s.register:
				s.mu.Lock()
				s.users[user.ID] = user
				s.mu.Unlock()
				fmt.Printf("User %s created\n\n", user.ID)

			case user := <-s.unregister:
				s.mu.Lock()
				if _, ok := s.users[user.ID]; ok {
					delete(s.users, user.ID)
					fmt.Printf("User %s disconnected\n", user.ID)
				}
				s.mu.Unlock()

			case message := <-s.broadcast:
				s.mu.Lock()
				if receiver, ok := s.users[message.To]; ok {
					if err := receiver.Conn.WriteJSON(message); err != nil {
						fmt.Printf("error sending message to %s: %s\n", message.To, err)
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}

func (s *Server) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "could not open websocket connection"+err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.URL.Query().Get("id")
	if userID == "" {
		http.Error(w, "missing user ID", http.StatusBadRequest)
		return
	}

	user := &User{ID: userID, Conn: conn}
	s.register <- user

	defer func() {
		s.unregister <- user
		conn.Close()
	}()

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("error reading JSON message from %s: %s\n", user.ID, err)
			break
		}
		msg.From = userID
		s.broadcast <- &msg
	}
}
