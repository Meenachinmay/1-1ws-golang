package main

import (
	"1-1ws/internal/chat"
	"log"
	"net/http"
)

func main() {
	server := chat.NewChatServer()
	server.Run()

	http.HandleFunc("/ws", server.WebSocketHandler)

	log.Println("starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
