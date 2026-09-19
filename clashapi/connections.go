package clashapi

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type ConnectionTracker struct {
	mu    sync.RWMutex
	conns map[string]*Connection

	upTotal   int64
	downTotal int64

	subs  map[chan struct{}]struct{}
	subMu sync.Mutex
}

func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		conns: make(map[string]*Connection),
		subs:  make(map[chan struct{}]struct{}),
	}
}

func (t *ConnectionTracker) Open(meta Metadata, chains []string) string {
	id := newID()
	c := &Connection{
		ID:       id,
		Metadata: meta,
		Start:    time.Now().Format(time.RFC3339),
		Chains:   chains,
	}
	t.mu.Lock()
	t.conns[id] = c
	t.mu.Unlock()
	t.notify()
	return id
}

func (t *ConnectionTracker) AddBytes(id string, up, down int64) {
	t.mu.Lock()
	if c, ok := t.conns[id]; ok {
		c.Upload += up
		c.Download += down
		t.upTotal += up
		t.downTotal += down
	}
	t.mu.Unlock()
}

func (t *ConnectionTracker) Close(id string) {
	t.mu.Lock()
	delete(t.conns, id)
	t.mu.Unlock()
	t.notify()
}

func (t *ConnectionTracker) Snapshot() ConnectionsResponse {
	t.mu.RLock()
	defer t.mu.RUnlock()
	list := make([]Connection, 0, len(t.conns))
	for _, c := range t.conns {
		list = append(list, *c)
	}
	return ConnectionsResponse{
		UploadTotal:   t.upTotal,
		DownloadTotal: t.downTotal,
		Connections:   list,
	}
}

func (t *ConnectionTracker) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	t.subMu.Lock()
	t.subs[ch] = struct{}{}
	t.subMu.Unlock()
	return ch, func() {
		t.subMu.Lock()
		delete(t.subs, ch)
		t.subMu.Unlock()
		close(ch)
	}
}

func (t *ConnectionTracker) notify() {
	t.subMu.Lock()
	defer t.subMu.Unlock()
	for ch := range t.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}