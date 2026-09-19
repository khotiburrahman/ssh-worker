package clashapi

import "sync"

type LogBroadcaster struct {
	mu   sync.RWMutex
	subs map[chan LogMessage]struct{}
}

func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{subs: make(map[chan LogMessage]struct{})}
}

func (b *LogBroadcaster) Publish(level, msg string) {
	lm := LogMessage{Type: level, Payload: msg}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- lm:
		default:
		}
	}
}

func (b *LogBroadcaster) Subscribe() (<-chan LogMessage, func()) {
	ch := make(chan LogMessage, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
		close(ch)
	}
}