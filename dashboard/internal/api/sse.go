package api

import (
	"fmt"
	"net/http"
)

type SSEBroker struct {
	clients    map[chan []byte]struct{}
	register   chan chan []byte
	unregister chan chan []byte
	broadcast  chan []byte
	stop       chan struct{}
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients:    make(map[chan []byte]struct{}),
		register:   make(chan chan []byte),
		unregister: make(chan chan []byte),
		broadcast:  make(chan []byte, 64),
		stop:       make(chan struct{}),
	}
}

func (b *SSEBroker) Run() {
	for {
		select {
		case client := <-b.register:
			b.clients[client] = struct{}{}
		case client := <-b.unregister:
			delete(b.clients, client)
			close(client)
		case msg := <-b.broadcast:
			for client := range b.clients {
				select {
				case client <- msg:
				default:
				}
			}
		case <-b.stop:
			for client := range b.clients {
				close(client)
			}
			return
		}
	}
}

func (b *SSEBroker) Stop() {
	close(b.stop)
}

func (b *SSEBroker) Broadcast(data []byte) {
	select {
	case b.broadcast <- data:
	case <-b.stop:
	}
}

func (b *SSEBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := make(chan []byte, 16)
	b.register <- client

	defer func() {
		select {
		case b.unregister <- client:
		case <-b.stop:
		}
	}()

	ctx := r.Context()
	for {
		select {
		case msg, ok := <-client:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}
