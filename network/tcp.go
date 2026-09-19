package network

import (
	"context"
	"net"
	"time"

	"github.com/youruser/gosshtunnel/internal/logger"
)

type TCPDialer struct {
	Address string
	Log     *logger.Logger
}

func (d *TCPDialer) DialContext(ctx context.Context) (net.Conn, error) {
	lg := d.Log.For(logger.CompNetwork)
	done := lg.Event("net.tcp.dial").Slow("tcp.dial", 3*time.Second)

	nd := net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	conn, err := nd.DialContext(ctx, "tcp", d.Address)
	done(err)
	if err != nil {
		lg.Errorf(logger.CodeNetDial, "tcp dial failed", err, "addr", d.Address)
		return nil, err
	}

	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
		_ = tc.SetNoDelay(true)
	}
	lg.Info("tcp connected",
		"addr", d.Address,
		"local", conn.LocalAddr().String(),
		"remote", conn.RemoteAddr().String(),
	)
	return conn, nil
}