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
	broadcast  chan *EventMessage
	register   chan *User
	unregister chan *User
	mu         sync.Mutex
}

func NewChatServer() *Server {
	return &Server{
		users:      make(map[string]*User),
		broadcast:  make(chan *EventMessage),
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
				s.broadcastUserList()
				s.mu.Unlock()
				fmt.Printf("User %s created\n\n", user.ID)

			case user := <-s.unregister:
				s.mu.Lock()
				if _, ok := s.users[user.ID]; ok {
					delete(s.users, user.ID)
					s.broadcastUserList()
					fmt.Printf("User %s disconnected\n", user.ID)
				}
				s.mu.Unlock()

			case message := <-s.broadcast:
				s.mu.Lock()
				msg, ok := message.Data.(*Message)
				if !ok {
					s.mu.Unlock()
					log.Printf("Invalid message received\n")
					continue
				}

				receiver, ok := s.users[msg.To]
				if !ok {
					// receiver is offline
					errMsg := &EventMessage{
						Event: "error",
						Data: map[string]string{
							"error": "receiver is offline",
							"from":  msg.From,
						},
					}
					sender, exist := s.users[msg.From]
					if exist {
						err := sender.Conn.WriteJSON(errMsg)
						if err != nil {
							fmt.Printf("error sending message to %s: %s\n", msg.From, err)
						}
					}
				} else {
					// Receiver exists, send the message
					newMsg := &EventMessage{
						Event: "new_message",
						Data: map[string]string{
							"from":     msg.From,
							"receiver": msg.To,
							"text":     msg.Text,
						},
					}
					if err := receiver.Conn.WriteJSON(newMsg); err != nil {
						fmt.Printf("error sending message to %s: %s\n", msg.To, err)
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}

func (s *Server) broadcastUserList() {
	userList := make([]string, 0, len(s.users))
	for id := range s.users {
		userList = append(userList, id)
	}

	listMessage := &EventMessage{
		Event: "user_list",
		Data:  userList,
	}

	for _, user := range s.users {
		if err := user.Conn.WriteJSON(listMessage); err != nil {
			log.Printf("error broadcasting user list: %s\n", err)
		}
	}
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
		var eventMessage EventMessage
		if err := conn.ReadJSON(&eventMessage); err != nil {
			log.Printf("error reading JSON message from %s: %s\n", user.ID, err)
			break
		}

		switch eventMessage.Event {
		case "new_message":
			var msg *Message
			if err := eventMessage.DecodeData(&msg); err != nil {
				log.Printf("error decoding message from %s: %s\n", user.ID, err)
				continue
			}

			msg.From = userID
			s.broadcast <- &EventMessage{
				Event: "new_message",
				Data:  msg,
			}
		}
	}
}
