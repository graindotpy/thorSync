package events

import (
	"encoding/json"
	"sync"
	"time"
)

type Event struct {
	Type      string    `json:"type"`
	GameID    string    `json:"gameId,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan []byte]struct{}
}

func New() *Hub { return &Hub{subscribers: map[chan []byte]struct{}{}} }

func (h *Hub) Publish(event Event) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	payload, _ := json.Marshal(event)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers {
		select {
		case subscriber <- payload:
		default:
		}
	}
}

func (h *Hub) Subscribe() (<-chan []byte, func()) {
	channel := make(chan []byte, 32)
	h.mu.Lock()
	h.subscribers[channel] = struct{}{}
	h.mu.Unlock()
	return channel, func() {
		h.mu.Lock()
		if _, ok := h.subscribers[channel]; ok {
			delete(h.subscribers, channel)
			close(channel)
		}
		h.mu.Unlock()
	}
}
