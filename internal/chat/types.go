package chat

import "github.com/gorilla/websocket"

type User struct {
	ID   string
	Conn *websocket.Conn
}

type Message struct {
	From string `json:"sender"`
	To   string `json:"receiver"`
	Text string `json:"text"`
}
