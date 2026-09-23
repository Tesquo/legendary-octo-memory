package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Hub is a fan-out broadcaster for progress events. Each subscriber gets its
// own buffered channel, so a slow client can never block the pipeline. The hub
// also remembers the most recent event per media item so that a client which
// connects mid-transcode immediately receives the current state.
//
// Progress is strictly one-way (server to browser), so it is exposed over
// Server-Sent Events rather than a full WebSocket: zero dependencies, automatic
// reconnection, and a plain HTTP endpoint.
type Hub struct {
	mu       sync.RWMutex
	clients  map[int]chan []byte
	nextID   int
	lastByID map[string][]byte
}

func NewHub() *Hub {
	return &Hub{
		clients:  make(map[int]chan []byte),
		lastByID: make(map[string][]byte),
	}
}

// Publish records and broadcasts the latest event for a media item. It is safe
// to call from multiple goroutines (the pipeline workers).
func (h *Hub) Publish(mediaID string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.PublishRaw(mediaID, data)
}

// PublishRaw is Publish for callers that have already marshalled the payload.
func (h *Hub) PublishRaw(mediaID string, data []byte) {
	h.mu.Lock()
	h.lastByID[mediaID] = data
	clients := make([]chan []byte, 0, len(h.clients))
	for _, ch := range h.clients {
		clients = append(clients, ch)
	}
	h.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- data:
		default:
			// Subscriber is too slow; drop this update rather than stall.
		}
	}
}

// Subscribe returns a channel of JSON event payloads plus an unsubscribe func.
// Callers must invoke the returned function when finished.
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	ch := make(chan []byte, 32)
	h.clients[id] = ch
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if c, ok := h.clients[id]; ok {
			delete(h.clients, id)
			close(c)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}

// Snapshot returns the most recent event for every media item.
func (h *Hub) Snapshot() [][]byte {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make([][]byte, 0, len(h.lastByID))
	for _, data := range h.lastByID {
		out = append(out, data)
	}
	return out
}

// ServeSSE streams progress events to a browser over Server-Sent Events.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// CORS is deliberately not set here: the router's cors middleware owns it
	// and derives the allowed origin from Config.AllowedOrigins. Hardcoding "*"
	// would override that and defeat the configuration.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := h.Subscribe()
	defer unsubscribe()

	// Bring a late-joining client up to date.
	for _, data := range h.Snapshot() {
		writeEvent(w, data)
	}
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			writeEvent(w, data)
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, data []byte) {
	fmt.Fprintf(w, "data: %s\n\n", data)
}
