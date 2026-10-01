package events

import (
	"encoding/json"
	"fmt"
	"sync"
)

type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan string]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan string]struct{}),
	}
}

func (b *Broker) Subscribe() chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan string, 100)
	b.subscribers[ch] = struct{}{}
	return ch
}

func (b *Broker) Unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.subscribers, ch)
	close(ch)
}

func (b *Broker) Publish(event interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("data: %s\n\n", string(payload))

	for ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			// If buffer is full, drop rather than blocking the publisher
		}
	}
}
