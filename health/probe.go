package health

import (
	"context"
	"net"
	"time"

	"github.com/youruser/gosshtunnel/internal/logger"
)

type ProbeFn func(ctx context.Context, network, addr string) (net.Conn, error)

type Probe struct {
	Target   string
	Interval time.Duration
	Timeout  time.Duration
	dialFn   ProbeFn
	tracker  *Tracker
	log      *logger.Logger
}

func NewProbe(dial ProbeFn, tracker *Tracker, log *logger.Logger) *Probe {
	return &Probe{
		Target:   "1.1.1.1:443",
		Interval: 3 * time.Second,
		Timeout:  5 * time.Second,
		dialFn:   dial,
		tracker:  tracker,
		log:      log.For(logger.CompHealth),
	}
}

func (p *Probe) Run(ctx context.Context) {
	tick := time.NewTicker(p.Interval)
	defer tick.Stop()

	p.once(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			p.once(ctx)
		}
	}
}

func (p *Probe) once(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, p.Timeout)
	defer cancel()

	start := time.Now()
	conn, err := p.dialFn(ctx, "tcp", p.Target)
	rtt := time.Since(start)

	if err != nil {
		p.tracker.ProbeFail(err)
		p.log.Warn("synthetic probe failed",
			"code", logger.CodeHltProbeFail,
			"target", p.Target, "dur", rtt.String(), "err", err,
		)
		return
	}
	_ = conn.Close()
	p.tracker.ProbeOK(rtt)
	p.log.Debug("synthetic probe ok",
		"target", p.Target, "rtt", rtt.String(),
	)
}