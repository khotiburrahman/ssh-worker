package qload

import (
	"context"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"github.com/youruser/gosshtunnel/internal/errs"
	"github.com/youruser/gosshtunnel/internal/logger"
)

type Config struct {
	MaxRetries       int
	Concurrency      int
	Timeout          time.Duration
	SemTimeout       time.Duration
	MinBackoff       time.Duration
	MaxBackoff       time.Duration
	FailureThreshold int32
	Cooldown         time.Duration
	AdaptiveTimeout  bool
}

type QLoad struct {
	cfg Config
	sem chan struct{}
	log *logger.Logger

	failures atomic.Int32
	openedAt atomic.Int64

	currentTimeout atomic.Int64
}

func New(cfg Config, log *logger.Logger) *QLoad {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 3
	}
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = 300 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 10 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.SemTimeout <= 0 {
		cfg.SemTimeout = 5 * time.Second
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 15 * time.Second
	}

	q := &QLoad{
		cfg: cfg,
		sem: make(chan struct{}, cfg.Concurrency),
		log: log.For(logger.CompQLoad),
	}
	q.currentTimeout.Store(int64(cfg.Timeout))
	return q
}

func (q *QLoad) Dial(
	ctx context.Context,
	fn func(context.Context) (net.Conn, error),
) (net.Conn, error) {
	lg := q.log.Event("qload.dial")

	if err := q.allow(); err != nil {
		lg.Errorf(logger.CodeQldBreaker, "circuit open", err,
			"failures", q.failures.Load())
		return nil, err
	}

	sctx, cancel := context.WithTimeout(ctx, q.cfg.SemTimeout)
	defer cancel()
	select {
	case q.sem <- struct{}{}:
		defer func() { <-q.sem }()
	case <-sctx.Done():
		lg.Warn("semaphore timeout",
			"code", logger.CodeQldBusy,
			"in_flight", len(q.sem))
		return nil, errs.ErrBusy
	}

	timeout := time.Duration(q.currentTimeout.Load())
	var lastErr error
	backoff := q.cfg.MinBackoff

	for i := 0; i <= q.cfg.MaxRetries; i++ {
		dctx, dcancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		conn, err := fn(dctx)
		dur := time.Since(start)
		dcancel()

		if err == nil {
			q.onSuccess()
			q.adaptTimeout(dur, true)
			lg.Debug("attempt ok", "attempt", i+1, "dur", dur.String())
			return conn, nil
		}
		lastErr = err
		q.adaptTimeout(dur, false)

		lg.Warn("attempt failed",
			"code", logger.CodeQldRetry,
			"attempt", i+1,
			"max", q.cfg.MaxRetries+1,
			"dur", dur.String(),
			"timeout", timeout.String(),
			"retryable", errs.IsRetryable(err),
			"err", err,
		)

		if !errs.IsRetryable(err) || i == q.cfg.MaxRetries {
			break
		}
		if !sleepJitter(ctx, backoff) {
			break
		}
		backoff = nextBackoff(backoff, q.cfg.MaxBackoff)
	}
	q.onFailure()
	lg.Errorf(logger.CodeQldRetry, "all retries exhausted", lastErr,
		"failures_total", q.failures.Load())
	return nil, lastErr
}

func (q *QLoad) adaptTimeout(last time.Duration, ok bool) {
	if !q.cfg.AdaptiveTimeout {
		return
	}
	cur := time.Duration(q.currentTimeout.Load())
	if ok {
		target := last * 2
		if target < 3*time.Second {
			target = 3 * time.Second
		}
		if target > q.cfg.Timeout {
			target = q.cfg.Timeout
		}
		next := cur - (cur-target)/4
		q.currentTimeout.Store(int64(next))
	} else {
		next := cur + cur/4
		if next > q.cfg.MaxBackoff {
			next = q.cfg.MaxBackoff
		}
		q.currentTimeout.Store(int64(next))
	}
}

func (q *QLoad) allow() error {
	opened := q.openedAt.Load()
	if opened == 0 {
		return nil
	}
	if time.Since(time.Unix(0, opened)) > q.cfg.Cooldown {
		if q.openedAt.CompareAndSwap(opened, 0) {
			q.failures.Store(0)
			return nil
		}
	}
	return errs.ErrCircuitOpen
}

func (q *QLoad) onSuccess() {
	if q.failures.Swap(0) > 0 || q.openedAt.Swap(0) != 0 {
		q.log.Event("qload.circuit.close").Info("circuit breaker CLOSED")
	}
}

func (q *QLoad) onFailure() {
	n := q.failures.Add(1)
	if n >= q.cfg.FailureThreshold {
		if q.openedAt.CompareAndSwap(0, time.Now().UnixNano()) {
			q.log.Event("qload.circuit.open").Warn("circuit breaker OPEN",
				"code", logger.CodeQldBreaker,
				"threshold", q.cfg.FailureThreshold,
				"cooldown", q.cfg.Cooldown.String(),
			)
		}
	}
}

func sleepJitter(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		d = 10 * time.Millisecond
	}
	j := time.Duration(rand.Int63n(int64(d/2 + 1)))
	t := time.NewTimer(d/2 + j)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func nextBackoff(cur, max time.Duration) time.Duration {
	n := cur * 2
	if n > max {
		n = max
	}
	return n
}