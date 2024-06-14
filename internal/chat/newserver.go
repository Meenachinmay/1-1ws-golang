package chat

import (
	"1-1ws/internal/websocket"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	users               sync.Map // Concurrent map for user connections
	broadcast           chan *EventMessage
	postNotify          chan *EventMessage
	register            chan *User
	unregister          chan *User
	workerPool          chan func()
	broadcastWorkerPool chan func()
	mu                  sync.Mutex
	Wg                  sync.WaitGroup
	connectionCount     int64
}

func NewChatServer(workerCount int) *Server {
	server := &Server{
		broadcast:           make(chan *EventMessage, 1000),
		postNotify:          make(chan *EventMessage, 1000),
		register:            make(chan *User, 1000),
		unregister:          make(chan *User, 1000),
		workerPool:          make(chan func(), 1000),
		broadcastWorkerPool: make(chan func(), 1000),
	}
	server.startWorkers(workerCount)
	server.startBroadcastWorkerPool(workerCount)
	return server
}

func (s *Server) startWorkers(count int) {
	for i := 0; i < count; i++ {
		go func() {
			for job := range s.workerPool {
				job()
			}
		}()
	}
}

func (s *Server) startBroadcastWorkerPool(count int) {
	for i := 0; i < count; i++ {
		go func() {
			for job := range s.broadcastWorkerPool {
				job()
			}
		}()
	}
}

func (user *User) SafeWriteJSON(v interface{}) error {
	user.mu.Lock()
	defer user.mu.Unlock()
	return user.Conn.WriteJSON(v)
}

func (s *Server) Run() {
	go func() {
		for {
			select {
			case user := <-s.register:
				s.handleUserRegistration(user)

			case user := <-s.unregister:
				s.handleUserUnregistration(user)

			case message := <-s.broadcast:
				msg, ok := message.Data.(*Message)
				if !ok {

					log.Printf("Invalid message received\n")
					continue
				}
				fmt.Printf("message := <- s.broadcast %v\n", msg)

				receiver, ok := s.users.Load(msg.To)
				if !ok {
					// receiver is offline
					errMsg := &EventMessage{
						Event: "error",
						Data: map[string]string{
							"error": "receiver is offline",
							"from":  msg.From,
						},
					}
					sender, exist := s.users.Load(msg.From)
					if exist {
						sender, ok := sender.(*User)
						if !ok {
							fmt.Printf("Type assertion failed for sender %s\n", msg.From)
							return // Or handle the error appropriately
						}
						err := sender.SafeWriteJSON(errMsg)
						if err != nil {
							fmt.Printf("error sending message to %s: %s\n", msg.From, err)
						}
					}
				} else {
					receiver, ok := receiver.(*User)
					if !ok {
						fmt.Printf("Type assertion failed for sender %s\n", msg.From)
						return // Or handle the error appropriately
					}
					// Receiver exists, send the message
					newMsg := &EventMessage{
						Event: "new_message",
						Data: map[string]string{
							"from":     msg.From,
							"receiver": msg.To,
							"text":     msg.Text,
						},
					}
					if err := receiver.SafeWriteJSON(newMsg); err != nil {
						fmt.Printf("error sending message to %s: %s\n", msg.To, err)
					}
				}

			case notification := <-s.postNotify:

				msg, ok := notification.Data.(*Message)
				if !ok {

					log.Printf("Invalid message received\n")
					continue
				}

				receiver, ok := s.users.Load(msg.To)
				if !ok {
					errMsg := &EventMessage{
						Event: "error",
						Data: map[string]string{
							"error": "receiver is offline",
							"from":  msg.From,
						},
					}
					sender, exist := s.users.Load(msg.From)
					if exist {
						sender, ok := sender.(*User)
						if !ok {
							fmt.Printf("Type assertion failed for sender %s\n", msg.From)
							return // Or handle the error appropriately
						}
						err := sender.SafeWriteJSON(errMsg)
						if err != nil {
							fmt.Printf("error sending message to %s: %s\n", msg.To, err)
						}
					}
				} else {
					receiver, ok := receiver.(*User)
					if !ok {
						fmt.Printf("Type assertion failed for sender %s\n", msg.From)
						return // Or handle the error appropriately
					}
					newNotification := &EventMessage{
						Event: "notify_user_about_post",
						Data: map[string]string{
							"from":     msg.From,
							"receiver": msg.To,
							"text":     msg.Text,
						},
					}
					if err := receiver.SafeWriteJSON(newNotification); err != nil {
						fmt.Printf("error sending notification to %s: %s\n", msg.To, err)
					}
				}

			}
		}
	}()
}

func (s *Server) handleUserRegistration(user *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users.Store(user.ID, user)
	connectionCount := atomic.AddInt64(&s.connectionCount, 1)
	if connectionCount == 10000 {
		log.Println("10000 connections reached. Broadcasting message in 2 seconds...")
		time.AfterFunc(2*time.Second, func() {
			s.broadcastToAll("broadcast", "true")
		})
	}
	fmt.Printf("User %s created\n\n", user.ID)
	s.Wg.Add(1)
}

func (s *Server) handleUserUnregistration(user *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users.Load(user.ID); ok {
		s.users.Delete(user.ID)
		atomic.AddInt64(&s.connectionCount, -1)
		fmt.Printf("User %s disconnected\n", user.ID)
	}
	s.Wg.Done()
}

func (s *Server) broadcastToAll(event, message string) {
	eventMessage := &EventMessage{
		Event: event,
		Data:  message,
	}

	var users []*User
	s.users.Range(func(_, value interface{}) bool {
		user, ok := value.(*User)
		if ok {
			users = append(users, user)
		}
		return true
	})

	// Split users into chunks and create jobs
	chunkSize := (len(users) + cap(s.broadcastWorkerPool) - 1) / cap(s.broadcastWorkerPool)
	for i := 0; i < len(users); i += chunkSize {
		end := i + chunkSize
		if end > len(users) {
			end = len(users)
		}
		userChunk := users[i:end]

		job := func() {
			for _, user := range userChunk {
				msg := fmt.Sprintf("UserID: %s, Message: %s", user.ID, eventMessage.Data)
				if err := user.SafeWriteJSON(msg); err != nil {
					log.Printf("error broadcasting %s event to %s: %s\n", event, user.ID, err)
				}
			}
		}

		s.broadcastWorkerPool <- job
	}
}

func (s *Server) broadcastUserList() {
	s.mu.Lock()
	defer s.mu.Unlock()
	userList := make([]string, 0)

	// Collect all user IDs from the sync.Map
	s.users.Range(func(key, value interface{}) bool {
		id, ok := key.(string)
		if ok {
			userList = append(userList, id)
		}
		return true // continue iterating
	})

	listMessage := &EventMessage{
		Event: "user_list",
		Data:  userList,
	}

	// Broadcast the user list to all connected users
	s.users.Range(func(_, value interface{}) bool {
		user, ok := value.(*User)
		if !ok {
			log.Println("Type assertion failed for User")
			return true // continue iterating
		}
		if err := user.Conn.WriteJSON(listMessage); err != nil {
			log.Printf("error broadcasting user list to %s: %s\n", user.ID, err)
		}
		return true // continue iterating
	})
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

		s.workerPool <- func() {
			switch eventMessage.Event {
			case "new_message":
				fmt.Println("new message event received.")
				var msg *Message
				if err := eventMessage.DecodeData(&msg); err != nil {
					log.Printf("error decoding message from %s: %s\n", user.ID, err)
					return
				}

				msg.From = userID
				s.broadcast <- &EventMessage{
					Event: "new_message",
					Data:  msg,
				}

			case "notify_user_about_post":
				fmt.Println("notify event received.")
				var msg *Message
				if err := eventMessage.DecodeData(&msg); err != nil {
					log.Printf("error decoding message from %s: %s\n", user.ID, err)
					return
				}

				msg.From = userID
				s.postNotify <- &EventMessage{
					Event: "notify_user_about_post",
					Data:  msg,
				}
			}
		}
	}
}

func (s *Server) Wait() {
	s.Wg.Wait()
}
