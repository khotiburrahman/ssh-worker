package socks5

import (
	"net"
	"sync"
	"time"
)

type idleConn struct {
	net.Conn
	timeout time.Duration
	lastAt  time.Time
	mu      sync.Mutex
	onKill  func(time.Duration)
	killed  bool
	stop    chan struct{}
}

func newIdleConn(c net.Conn, timeout time.Duration, onKill func(time.Duration)) net.Conn {
	if timeout <= 0 {
		return c
	}
	ic := &idleConn{
		Conn:    c,
		timeout: timeout,
		lastAt:  time.Now(),
		onKill:  onKill,
		stop:    make(chan struct{}),
	}
	go ic.watch()
	return ic
}

func (c *idleConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func (c *idleConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func (c *idleConn) Close() error {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	return c.Conn.Close()
}

func (c *idleConn) touch() {
	c.mu.Lock()
	c.lastAt = time.Now()
	c.mu.Unlock()
}

func (c *idleConn) watch() {
	tick := time.NewTicker(c.timeout / 2)
	defer tick.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-tick.C:
			c.mu.Lock()
			idle := time.Since(c.lastAt)
			c.mu.Unlock()
			if idle >= c.timeout {
				c.mu.Lock()
				if c.killed {
					c.mu.Unlock()
					return
				}
				c.killed = true
				c.mu.Unlock()
				if c.onKill != nil {
					c.onKill(idle)
				}
				_ = c.Conn.Close()
				return
			}
		}
	}
}