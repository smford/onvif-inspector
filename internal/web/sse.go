package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/smford/onvif-inspector/internal/hub"
)

// SSEBroker manages connected Server-Sent Events clients
type SSEBroker struct {
	hub         *hub.Hub
	mu          sync.RWMutex
	clients     map[chan []byte]struct{}
	stopCh      chan struct{}
	unsubEvents func()
	unsubDevs   func()
}

// NewSSEBroker initializes an SSE broadcaster connected to the central hub
func NewSSEBroker(h *hub.Hub) *SSEBroker {
	b := &SSEBroker{
		hub:     h,
		clients: make(map[chan []byte]struct{}),
		stopCh:  make(chan struct{}),
	}
	b.start()
	return b
}

func (b *SSEBroker) start() {
	eventCh, unsubEv := b.hub.SubscribeEvents(200)
	devCh, unsubDev := b.hub.SubscribeDevices(50)
	b.unsubEvents = unsubEv
	b.unsubDevs = unsubDev

	go func() {
		for {
			select {
			case <-b.stopCh:
				return

			case ev, ok := <-eventCh:
				if !ok {
					return
				}
				data, err := json.Marshal(ev)
				if err == nil {
					msg := fmt.Sprintf("event: onvif-event\ndata: %s\n\n", data)
					b.broadcast([]byte(msg))
				}

			case dev, ok := <-devCh:
				if !ok {
					return
				}
				data, err := json.Marshal(dev)
				if err == nil {
					msg := fmt.Sprintf("event: device-update\ndata: %s\n\n", data)
					b.broadcast([]byte(msg))
				}
			}
		}
	}()
}

// Close terminates the broker and unregisters hub subscriptions
func (b *SSEBroker) Close() {
	select {
	case <-b.stopCh:
		return // already closed
	default:
		close(b.stopCh)
	}

	if b.unsubEvents != nil {
		b.unsubEvents()
	}
	if b.unsubDevs != nil {
		b.unsubDevs()
	}

	b.mu.Lock()
	for ch := range b.clients {
		close(ch)
	}
	b.clients = make(map[chan []byte]struct{})
	b.mu.Unlock()
}

func (b *SSEBroker) broadcast(msg []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// Non-blocking drop if client is saturated
		}
	}
}

func (b *SSEBroker) addClient(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[ch] = struct{}{}
}

func (b *SSEBroker) removeClient(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
}

// ServeHTTP handles the SSE streaming endpoint /api/events/stream
func (b *SSEBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientCh := make(chan []byte, 50)
	b.addClient(clientCh)
	defer b.removeClient(clientCh)

	// Send initial ping
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	// Keepalive ticker to prevent intermediate proxy timeouts
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-b.stopCh:
			return
		case msg, ok := <-clientCh:
			if !ok {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
