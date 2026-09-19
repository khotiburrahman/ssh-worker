package socks5

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armon/go-socks5"
	"github.com/youruser/gosshtunnel/internal/logger"
	"github.com/youruser/gosshtunnel/internal/rules"
)

type DialFunc func(network, addr string) (net.Conn, error)

type Options struct {
	Listen      string
	MaxConns    int
	IdleTimeout time.Duration
	HandshakeTO time.Duration
	Log         *logger.Logger
	RuleEngine  *rules.Engine
	DirectDial  DialFunc
}

type Server struct {
	opt   Options
	inner *socks5.Server
	ln    net.Listener
	log   *logger.Logger

	active atomic.Int64
	total  atomic.Int64
	wg     sync.WaitGroup

	closeOnce sync.Once
}

func New(opt Options, dial DialFunc) (*Server, error) {
	if opt.MaxConns <= 0 {
		opt.MaxConns = 1024
	}
	if opt.IdleTimeout <= 0 {
		opt.IdleTimeout = 5 * time.Minute
	}
	if opt.HandshakeTO <= 0 {
		opt.HandshakeTO = 15 * time.Second
	}

	s := &Server{opt: opt, log: opt.Log.For(logger.CompSOCKS5)}

	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return s.route(network, addr, dial)
		},
	}
	inner, err := socks5.New(conf)
	if err != nil {
		return nil, err
	}
	s.inner = inner
	return s, nil
}

// route = intercept rule engine sebelum dial ke SSH / direct.
func (s *Server) route(network, addr string, sshDial DialFunc) (net.Conn, error) {
	if s.opt.RuleEngine == nil {
		return sshDial(network, addr)
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port := atoiOrZero(portStr)

	mctx := rules.MatchContext{
		Host:    rules.NormalizeHost(host),
		Port:    port,
		Network: network,
	}
	if ip := net.ParseIP(host); ip != nil {
		mctx.IP = ip
		mctx.Host = ""
	}

	res := s.opt.RuleEngine.Match(mctx)

	lg := s.log.With("dest", addr, "host", mctx.Host, "port", port)
	switch res.Adapter {
	case rules.AdapterReject:
		lg.Warn("blocked by rule",
			"code", logger.CodeRulBlocked,
			"rule", res.MatchedBy,
		)
		return nil, fmt.Errorf("blocked by rule: %s", res.MatchedBy)
	case rules.AdapterDirect:
		lg.Debug("direct via rule", "rule", res.MatchedBy)
		if s.opt.DirectDial != nil {
			return s.opt.DirectDial(network, addr)
		}
		return net.Dial(network, addr)
	default:
		lg.Debug("proxied via rule",
			"rule", res.MatchedBy, "adapter", res.Adapter)
		return sshDial(network, addr)
	}
}

func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.opt.Listen)
	if err != nil {
		s.log.Errorf(logger.CodeSKSServe, "listen failed", err,
			"addr", s.opt.Listen)
		return err
	}
	s.ln = ln
	s.log.Info("socks5 listening",
		"addr", s.opt.Listen,
		"max_conns", s.opt.MaxConns,
		"handshake_to", s.opt.HandshakeTO.String(),
		"rules", s.opt.RuleEngine != nil,
	)

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				s.drain()
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				s.drain()
				return nil
			}
			s.log.Warn("accept error",
				"code", logger.CodeSKSAccept, "err", err)
			continue
		}

		if int(s.active.Load()) >= s.opt.MaxConns {
			s.log.Warn("conn limit reached",
				"code", logger.CodeSKSLimit,
				"active", s.active.Load(), "max", s.opt.MaxConns,
			)
			_ = conn.Close()
			continue
		}

		id := s.total.Add(1)
		s.active.Add(1)
		s.wg.Add(1)
		go s.handleConn(id, conn)
	}
}

func (s *Server) handleConn(id int64, conn net.Conn) {
	defer s.wg.Done()
	defer s.active.Add(-1)
	defer conn.Close()
	defer s.log.PanicRecover("socks5.handle")

	start := time.Now()
	remote := conn.RemoteAddr().String()
	lg := s.log.With("conn_id", id, "remote", remote)

	_ = conn.SetDeadline(time.Now().Add(s.opt.HandshakeTO))

	wrapped := newIdleConn(conn, s.opt.IdleTimeout, func(idle time.Duration) {
		lg.Warn("conn idle di-kill",
			"code", logger.CodeHltIdleKill,
			"idle", idle.String(), "dur", time.Since(start).String(),
		)
	})

	if err := s.inner.ServeConn(wrapped); err != nil {
		lg.Debug("conn closed",
			"dur", time.Since(start).String(), "err", err)
	} else {
		lg.Debug("conn done", "dur", time.Since(start).String())
	}
}

func (s *Server) drain() {
	lg := s.log.Event("socks5.drain")
	active := s.active.Load()
	lg.Info("draining",
		"code", logger.CodeSKSDrain,
		"active", active, "total", s.total.Load())

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		lg.Info("drained cleanly")
	case <-time.After(10 * time.Second):
		lg.Warn("drain timeout — forcing exit",
			"still_active", s.active.Load())
	}
}

func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		if s.ln != nil {
			_ = s.ln.Close()
		}
	})
	return nil
}

func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}