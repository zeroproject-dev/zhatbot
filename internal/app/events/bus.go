package events

import (
	"log"
	"sync"
)

const (
	TopicChatMessage  = "chat:message"
	TopicNotification = "notifications:event"
	TopicAppError     = "app:error"
	TopicStreamStatus = "stream:status"
	TopicTTSStatus    = "tts:status"
	TopicTTSSpoken    = "tts:spoken"

	defaultBufferSize = 128
)

type Bus struct {
	mu        sync.RWMutex
	subs      map[string]map[int]chan any
	nextSubID int
	closed    bool

	dropMu     sync.Mutex
	dropCounts map[string]uint64
}

func NewBus() *Bus {
	return &Bus{
		subs:       make(map[string]map[int]chan any),
		dropCounts: make(map[string]uint64),
	}
}

func (b *Bus) Publish(topic string, payload any) {
	if topic == "" {
		return
	}
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	channels := make([]chan any, 0, len(b.subs[topic]))
	for _, ch := range b.subs[topic] {
		channels = append(channels, ch)
	}
	b.mu.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- payload:
		default:
			b.recordDrop(topic)
		}
	}
}

func (b *Bus) Subscribe(topic string) (<-chan any, func()) {
	ch := make(chan any, defaultBufferSize)

	b.mu.Lock()
	if b.subs == nil {
		b.subs = make(map[string]map[int]chan any)
	}
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[int]chan any)
	}
	id := b.nextSubID
	b.nextSubID++
	b.subs[topic][id] = ch
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subs, ok := b.subs[topic]; ok {
			delete(subs, id)
			if len(subs) == 0 {
				delete(b.subs, topic)
			}
		}
		close(ch)
	}

	return ch, unsubscribe
}

func (b *Bus) recordDrop(topic string) {
	b.dropMu.Lock()
	defer b.dropMu.Unlock()
	if b.dropCounts == nil {
		b.dropCounts = make(map[string]uint64)
	}
	b.dropCounts[topic]++
	if b.dropCounts[topic]%100 == 1 {
		log.Printf("events: dropping messages for %s (total drops: %d)", topic, b.dropCounts[topic])
	}
}
