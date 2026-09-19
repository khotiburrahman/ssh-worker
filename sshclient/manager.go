package sshclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/youruser/gosshtunnel/config"
	"github.com/youruser/gosshtunnel/health"
	"github.com/youruser/gosshtunnel/internal/errs"
	"github.com/youruser/gosshtunnel/internal/logger"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Manager struct {
	cfg            config.SSHConfig
	knownHostsPath string
	dialFn         func(ctx context.Context) (net.Conn, error)
	log            *logger.Logger

	minBackoff  time.Duration
	maxBackoff  time.Duration
	keepAlive   time.Duration
	keepTimeout time.Duration

	state atomic.Int32

	// client & sshConn dilindungi mutex karena keduanya interface
	// (bukan pointer), jadi tidak bisa pakai atomic.Pointer.
	connMu  sync.RWMutex
	client  *ssh.Client
	sshConn ssh.Conn

	tracker *health.Tracker
	probe   *health.Probe

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewManager(
	parent context.Context,
	cfg config.SSHConfig,
	knownHostsPath string,
	dialFn func(ctx context.Context) (net.Conn, error),
	healthCfg health.Config,
	log *logger.Logger,
) *Manager {
	ctx, cancel := context.WithCancel(parent)
	m := &Manager{
		cfg:            cfg,
		knownHostsPath: knownHostsPath,
		dialFn:         dialFn,
		log:            log.For(logger.CompSSH),
		minBackoff:     1 * time.Second,
		maxBackoff:     30 * time.Second,
		keepAlive:      30 * time.Second,
		keepTimeout:    10 * time.Second,
		ctx:            ctx,
		cancel:         cancel,
	}
	m.tracker = health.New(healthCfg, log)

	// ProbeFn = (ctx, network, addr); m.Dial = (network, addr).
	// Bungkus dengan goroutine agar ctx deadline tetap dihormati.
	m.probe = health.NewProbe(func(pctx context.Context, network, addr string) (net.Conn, error) {
		type res struct {
			conn net.Conn
			err  error
		}
		ch := make(chan res, 1)
		go func() {
			c, e := m.Dial(network, addr)
			ch <- res{c, e}
		}()
		select {
		case r := <-ch:
			return r.conn, r.err
		case <-pctx.Done():
			return nil, pctx.Err()
		}
	}, m.tracker, log)

	m.tracker.OnDegrade(func(reason string) {
		m.log.Warn("auto-heal: paksa soft reconnect",
			"code", logger.CodeHltDegradedHold,
			"reason", reason,
		)
		m.tracker.IncReconnect()
		m.markDown()
	})
	m.tracker.OnUnhealthy(func(reason string) {
		m.log.Error("worker tidak sehat — reconnect paksa",
			"code", logger.CodeHltUnhealthy,
			"reason", reason,
		)
		m.tracker.IncReconnect()
		m.markDown()
	})
	m.tracker.OnRecover(func() {
		m.log.Info("worker kembali sehat",
			"code", logger.CodeHltRecovered,
		)
	})

	return m
}

func (m *Manager) Health() *health.Tracker { return m.tracker }
func (m *Manager) Healthy() bool           { return m.tracker.Healthy() }
func (m *Manager) State() State            { return State(m.state.Load()) }

// ---------- accessor thread-safe ----------

func (m *Manager) getClient() *ssh.Client {
	m.connMu.RLock()
	defer m.connMu.RUnlock()
	return m.client
}

func (m *Manager) getSSHConn() ssh.Conn {
	m.connMu.RLock()
	defer m.connMu.RUnlock()
	return m.sshConn
}

func (m *Manager) setConn(c *ssh.Client, conn ssh.Conn) {
	m.connMu.Lock()
	m.client = c
	m.sshConn = conn
	m.connMu.Unlock()
}

func (m *Manager) clearConn() (c *ssh.Client, conn ssh.Conn) {
	m.connMu.Lock()
	c, conn = m.client, m.sshConn
	m.client, m.sshConn = nil, nil
	m.connMu.Unlock()
	return
}

// ---------- lifecycle ----------

func (m *Manager) Start() error {
	lg := m.log.Event("ssh.start")
	lg.Info("connecting", "host", m.cfg.Host, "port", m.cfg.Port, "user", m.cfg.Username)
	if err := m.connect(m.ctx); err != nil {
		return err
	}
	m.wg.Add(2)
	go m.supervise()
	go func() {
		defer m.wg.Done()
		defer m.log.PanicRecover("ssh.health_probe")
		m.probe.Run(m.ctx)
	}()
	return nil
}

func (m *Manager) Dial(network, addr string) (net.Conn, error) {
	lg := m.log.Event("ssh.dial_dest")
	c := m.getClient()
	if c == nil || State(m.state.Load()) != StateUp {
		lg.Warn("dial rejected",
			"code", logger.CodeSSHDialDest,
			"reason", "not connected", "state", State(m.state.Load()).String(),
			"dest", addr)
		m.tracker.Observe(0, errs.ErrNotConnected)
		return nil, errs.ErrNotConnected
	}
	start := time.Now()
	conn, err := c.Dial(network, addr)
	dur := time.Since(start)
	m.tracker.Observe(dur, err)
	if err != nil {
		lg.Errorf(logger.CodeSSHDialDest, "dest dial failed", err,
			"dest", addr, "dur", dur.String())
		go m.markDown()
		return nil, err
	}
	lg.Debug("dest dial ok", "dest", addr, "dur", dur.String())
	return conn, nil
}

func (m *Manager) Close() error {
	m.log.Event("ssh.close").Info("shutting down")
	m.cancel()
	m.wg.Wait()
	m.markDown()
	return nil
}

// ---------- internal ----------

func (m *Manager) connect(ctx context.Context) error {
	lg := m.log.Event("ssh.connect")
	if !m.state.CompareAndSwap(int32(StateDown), int32(StateConnecting)) {
		lg.Debug("connect skipped — state not down",
			"state", State(m.state.Load()).String())
		return nil
	}
	lg.Info("dialing via transport pipeline")
	done := lg.Slow("ssh.connect_total", 45*time.Second)
	ok := false
	defer func() {
		if !ok {
			m.state.Store(int32(StateDown))
		}
	}()

	dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	raw, err := m.dialFn(dctx)
	if err != nil {
		done(err)
		lg.Errorf(logger.CodeSSHConnect, "transport dial failed", err)
		return fmt.Errorf("dial: %w", err)
	}

	cfg, err := m.buildSSHConfig(lg)
	if err != nil {
		done(err)
		_ = raw.Close()
		lg.Errorf(logger.CodeSSHAuth, "ssh config build failed", err)
		return err
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(raw, m.cfg.Host, cfg)
	if err != nil {
		done(err)
		_ = raw.Close()
		lg.Errorf(logger.CodeSSHHandshake, "ssh handshake failed", err,
			"host", m.cfg.Host, "user", m.cfg.Username)
		return fmt.Errorf("handshake: %w", err)
	}

	m.setConn(ssh.NewClient(sshConn, chans, reqs), sshConn)
	m.state.Store(int32(StateUp))
	ok = true
	done(nil)
	lg.Event("ssh.connected").Info("ssh connection established",
		"code", logger.CodeSSHConnected,
		"host", m.cfg.Host, "user", m.cfg.Username,
		"server_version", string(sshConn.ServerVersion()),
	)
	return nil
}

func (m *Manager) buildSSHConfig(lg *logger.Logger) (*ssh.ClientConfig, error) {
	var auths []ssh.AuthMethod
	if m.cfg.Password != "" {
		auths = append(auths, ssh.Password(m.cfg.Password))
		lg.Debug("auth method: password")
	}
	if m.cfg.PrivateKey != "" {
		keyData, err := os.ReadFile(m.cfg.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("baca private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
		lg.Debug("auth method: publickey", "file", m.cfg.PrivateKey)
	}
	if len(auths) == 0 {
		return nil, errors.New("ssh: tidak ada metode auth")
	}

	hk, err := m.hostKeyCallback(lg)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User:            m.cfg.Username,
		Auth:            auths,
		HostKeyCallback: hk,
		Timeout:         30 * time.Second,
	}, nil
}

func (m *Manager) hostKeyCallback(lg *logger.Logger) (ssh.HostKeyCallback, error) {
	if m.knownHostsPath == "" {
		lg.Warn("known_hosts kosong — memakai InsecureIgnoreHostKey (DEV ONLY)")
		return ssh.InsecureIgnoreHostKey(), nil
	}
	cb, err := knownhosts.New(m.knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	lg.Debug("host key verification enabled", "file", m.knownHostsPath)
	return cb, nil
}

func (m *Manager) markDown() {
	if !m.state.CompareAndSwap(int32(StateUp), int32(StateDown)) {
		return
	}
	m.log.Event("ssh.down").Warn("ssh marked down",
		"code", logger.CodeSSHDown)
	c, conn := m.clearConn()
	if c != nil {
		_ = c.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

func (m *Manager) supervise() {
	defer m.wg.Done()
	defer m.log.PanicRecover("ssh.supervisor")

	backoff := m.minBackoff
	consecutiveFails := 0

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		if State(m.state.Load()) == StateUp {
			err := m.keepaliveOnce(m.ctx)
			if err == nil {
				backoff = m.minBackoff
				consecutiveFails = 0
				if !m.sleep(m.keepAlive) {
					return
				}
				continue
			}
			if !errors.Is(err, context.Canceled) {
				m.log.Event("ssh.keepalive.fail").
					Errorf(logger.CodeSSHKeepalive, "keepalive failed", err,
						"consecutive_fails", consecutiveFails+1)
				m.markDown()
			}
		}

		if !m.sleepJitter(backoff) {
			return
		}
		consecutiveFails++
		m.log.Event("ssh.reconnect").Info("reconnect attempt",
			"code", logger.CodeSSHReconnect,
			"backoff", backoff.String(),
			"attempt", consecutiveFails,
		)
		if err := m.connect(m.ctx); err != nil {
			backoff = nextBackoff(backoff, m.maxBackoff)
			m.log.Warn("reconnect failed",
				"code", logger.CodeSSHReconnect,
				"err", err,
				"next_backoff", backoff.String(),
				"consecutive", consecutiveFails,
			)
			continue
		}
		backoff = m.minBackoff
		consecutiveFails = 0
	}
}

func (m *Manager) keepaliveOnce(ctx context.Context) error {
	c := m.getSSHConn()
	if c == nil {
		return errs.ErrNotConnected
	}
	tctx, cancel := context.WithTimeout(ctx, m.keepTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()
	select {
	case <-tctx.Done():
		return tctx.Err()
	case err := <-done:
		return err
	}
}

func (m *Manager) sleep(d time.Duration) bool {
	select {
	case <-m.ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func (m *Manager) sleepJitter(d time.Duration) bool {
	if d <= 0 {
		return m.sleep(0)
	}
	jitter := time.Duration(rand.Int63n(int64(d/2 + 1)))
	return m.sleep(d/2 + jitter)
}

func nextBackoff(cur, max time.Duration) time.Duration {
	n := cur * 2
	if n > max {
		n = max
	}
	return n
}