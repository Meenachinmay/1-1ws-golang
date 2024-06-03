package main

import (
	"1-1ws/internal/chat"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	server := chat.NewChatServer(100)
	go func() {
		server.Run()
	}()

	http.HandleFunc("/ws", server.WebSocketHandler)

	log.Println("Starting server on :8081")
	go func() {
		if err := http.ListenAndServe(":8081", nil); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Setting up a channel to listen for interrupt or termination signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Block until a signal is received
	<-stop

	log.Println("Shutting down server...")

	// Create a context to attempt a graceful shutdown within a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shut down any other services or handlers here, if necessary

	go func() {
		// Wait for server operations to finish
		server.Wait()
		cancel() // Cancel context once server has finished
	}()

	// Check if the context deadline is exceeded
	<-ctx.Done()
	switch ctx.Err() {
	case context.DeadlineExceeded:
		log.Println("Shutdown timed out, forcing exit.")
	default:
		log.Println("Server shutdown gracefully.")
	}
}
