package health

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/youruser/gosshtunnel/internal/logger"
)

type Config struct {
	DegradedRTT  time.Duration
	UnhealthyRTT time.Duration
	DegradedErr  float64
	UnhealthyErr float64
	WindowSize   int
	DegradedHold time.Duration
	HealHold     time.Duration
}

type Snapshot struct {
	State         State         `json:"state"`
	RTTEWMA       time.Duration `json:"rtt_ewma_ms"`
	P95           time.Duration `json:"p95_ms"`
	ErrRate       float64       `json:"err_rate"`
	Samples       int           `json:"samples"`
	LastProbe     time.Time     `json:"last_probe"`
	ProbeOK       int64         `json:"probe_ok_total"`
	ProbeFail     int64         `json:"probe_fail_total"`
	DegradedSince time.Time     `json:"degraded_since,omitempty"`
	Reconnects    int64         `json:"reconnects"`
}

type Tracker struct {
	cfg Config
	log *logger.Logger

	mu      sync.Mutex
	window  []float64
	idx     int
	count   int
	ewmaRTT float64
	ewmaErr float64

	state         atomic.Int32
	degradedSince time.Time
	healthySince  time.Time

	lastProbe  atomic.Int64
	probeOK    atomic.Int64
	probeFail  atomic.Int64
	reconnects atomic.Int64

	onDegrade   func(reason string)
	onUnhealthy func(reason string)
	onRecover   func()
}

func New(cfg Config, log *logger.Logger) *Tracker {
	if cfg.DegradedRTT <= 0 {
		cfg.DegradedRTT = 800 * time.Millisecond
	}
	if cfg.UnhealthyRTT <= 0 {
		cfg.UnhealthyRTT = 2000 * time.Millisecond
	}
	if cfg.DegradedErr <= 0 {
		cfg.DegradedErr = 0.02
	}
	if cfg.UnhealthyErr <= 0 {
		cfg.UnhealthyErr = 0.10
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 50
	}
	if cfg.DegradedHold <= 0 {
		cfg.DegradedHold = 5 * time.Second
	}
	if cfg.HealHold <= 0 {
		cfg.HealHold = 10 * time.Second
	}
	t := &Tracker{
		cfg:    cfg,
		log:    log.For(logger.CompHealth),
		window: make([]float64, cfg.WindowSize),
	}
	t.state.Store(int32(StateHealthy))
	t.healthySince = time.Now()
	return t
}

func (t *Tracker) OnDegrade(fn func(string))   { t.onDegrade = fn }
func (t *Tracker) OnUnhealthy(fn func(string)) { t.onUnhealthy = fn }
func (t *Tracker) OnRecover(fn func())         { t.onRecover = fn }

func (t *Tracker) Observe(dur time.Duration, err error) {
	t.mu.Lock()
	ms := float64(dur.Microseconds()) / 1000.0
	t.window[t.idx] = ms
	t.idx = (t.idx + 1) % t.cfg.WindowSize
	if t.count < t.cfg.WindowSize {
		t.count++
	}

	const alpha = 0.2
	if t.ewmaRTT == 0 {
		t.ewmaRTT = ms
	} else {
		t.ewmaRTT = alpha*ms + (1-alpha)*t.ewmaRTT
	}

	var e float64
	if err != nil {
		e = 1
	}
	t.ewmaErr = alpha*e + (1-alpha)*t.ewmaErr
	t.mu.Unlock()

	t.evaluate()
}

func (t *Tracker) ProbeOK(rtt time.Duration) {
	t.lastProbe.Store(time.Now().UnixNano())
	t.probeOK.Add(1)
	t.Observe(rtt, nil)
}

func (t *Tracker) ProbeFail(err error) {
	t.lastProbe.Store(time.Now().UnixNano())
	t.probeFail.Add(1)
	t.Observe(0, err)
}

func (t *Tracker) IncReconnect() { t.reconnects.Add(1) }

func (t *Tracker) State() State   { return State(t.state.Load()) }
func (t *Tracker) Healthy() bool  { return t.State() == StateHealthy }

func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	rtt := time.Duration(t.ewmaRTT * float64(time.Millisecond))
	errRate := t.ewmaErr
	samples := t.count
	p95 := t.percentile(0.95)
	t.mu.Unlock()

	last := time.Time{}
	if v := t.lastProbe.Load(); v > 0 {
		last = time.Unix(0, v)
	}
	return Snapshot{
		State:         t.State(),
		RTTEWMA:       rtt,
		P95:           time.Duration(p95 * float64(time.Millisecond)),
		ErrRate:       errRate,
		Samples:       samples,
		LastProbe:     last,
		ProbeOK:       t.probeOK.Load(),
		ProbeFail:     t.probeFail.Load(),
		DegradedSince: t.degradedSince,
		Reconnects:    t.reconnects.Load(),
	}
}

func (t *Tracker) evaluate() {
	t.mu.Lock()
	p95 := t.percentile(0.95)
	errRate := t.ewmaErr
	t.mu.Unlock()

	cur := t.State()
	now := time.Now()

	degraded := p95 > float64(t.cfg.DegradedRTT.Milliseconds()) || errRate > t.cfg.DegradedErr
	unhealthy := p95 > float64(t.cfg.UnhealthyRTT.Milliseconds()) || errRate > t.cfg.UnhealthyErr

	switch cur {
	case StateHealthy:
		if unhealthy {
			t.transition(StateUnhealthy, "threshold unhealthy terlampaui", p95, errRate)
		} else if degraded {
			t.transition(StateDegraded, "threshold degraded terlampaui", p95, errRate)
		}

	case StateDegraded:
		if unhealthy {
			t.transition(StateUnhealthy, "naik ke unhealthy", p95, errRate)
			return
		}
		if !degraded {
			if t.healthySince.IsZero() {
				t.healthySince = now
			} else if now.Sub(t.healthySince) >= t.cfg.HealHold {
				t.transition(StateHealthy, "kembali normal", p95, errRate)
			}
		} else {
			t.healthySince = time.Time{}
			if !t.degradedSince.IsZero() &&
				now.Sub(t.degradedSince) >= t.cfg.DegradedHold {
				t.log.Warn("degraded hold terlampaui — trigger heal",
					"code", logger.CodeHltDegradedHold,
					"hold", t.cfg.DegradedHold.String(),
					"p95_ms", p95, "err_rate", errRate,
				)
				if t.onDegrade != nil {
					go t.onDegrade("hold exceeded")
				}
				t.degradedSince = now
			}
		}

	case StateUnhealthy:
		if !degraded && !unhealthy {
			if t.healthySince.IsZero() {
				t.healthySince = now
			} else if now.Sub(t.healthySince) >= t.cfg.HealHold {
				t.transition(StateHealthy, "pulih dari unhealthy", p95, errRate)
			}
		} else {
			t.healthySince = time.Time{}
		}

	case StateRecovering:
		if !degraded && !unhealthy {
			t.transition(StateHealthy, "recovery selesai", p95, errRate)
		} else if unhealthy {
			t.transition(StateUnhealthy, "recovery gagal", p95, errRate)
		}
	}
}

func (t *Tracker) transition(to State, reason string, p95, errRate float64) {
	from := t.State()
	if from == to {
		return
	}
	t.state.Store(int32(to))

	switch to {
	case StateDegraded:
		t.degradedSince = time.Now()
		t.log.Warn("state → DEGRADED",
			"code", logger.CodeHltDegraded,
			"reason", reason,
			"p95_ms", p95, "err_rate", errRate,
			"threshold_rtt_ms", t.cfg.DegradedRTT.Milliseconds(),
		)
		if t.onDegrade != nil {
			go t.onDegrade(reason)
		}
	case StateUnhealthy:
		t.log.Error("state → UNHEALTHY",
			"code", logger.CodeHltUnhealthy,
			"reason", reason,
			"p95_ms", p95, "err_rate", errRate,
		)
		if t.onUnhealthy != nil {
			go t.onUnhealthy(reason)
		}
	case StateHealthy:
		t.healthySince = time.Now()
		t.log.Info("state → HEALTHY",
			"code", logger.CodeHltRecovered,
			"reason", reason,
			"p95_ms", p95, "err_rate", errRate,
		)
		if t.onRecover != nil {
			go t.onRecover()
		}
	}
}

func (t *Tracker) percentile(p float64) float64 {
	if t.count == 0 {
		return 0
	}
	tmp := make([]float64, t.count)
	copy(tmp, t.window[:t.count])
	for i := 1; i < len(tmp); i++ {
		for j := i; j > 0 && tmp[j-1] > tmp[j]; j-- {
			tmp[j-1], tmp[j] = tmp[j], tmp[j-1]
		}
	}
	idx := int(math.Ceil(p*float64(len(tmp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(tmp) {
		idx = len(tmp) - 1
	}
	return tmp[idx]
}