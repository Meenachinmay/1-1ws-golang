package chat

import (
	"encoding/json"
	"github.com/gorilla/websocket"
)

type User struct {
	ID   string
	Conn *websocket.Conn
}

type EventMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

type Message struct {
	From string `json:"sender"`
	To   string `json:"receiver"`
	Text string `json:"text"`
}

func (e *EventMessage) DecodeData(target interface{}) error {
	// marshal
	dataBytes, err := json.Marshal(e.Data)
	if err != nil {
		return err
	}

	// unmarshal
	return json.Unmarshal(dataBytes, target)
}
