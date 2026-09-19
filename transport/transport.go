package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/youruser/gosshtunnel/config"
	"github.com/youruser/gosshtunnel/internal/errs"
	"github.com/youruser/gosshtunnel/internal/logger"
	"github.com/youruser/gosshtunnel/network"
)

type Transport struct {
	dialer network.Dialer
	cfg    *config.Config
	log    *logger.Logger
}

func New(d network.Dialer, cfg *config.Config, log *logger.Logger) *Transport {
	return &Transport{dialer: d, cfg: cfg, log: log.For(logger.CompTransport)}
}

func (t *Transport) Dial(ctx context.Context) (net.Conn, error) {
	lg := t.log.Event("transport.dial")

	// Stage 1: network
	conn, err := t.dialer.DialContext(ctx)
	if err != nil {
		lg.Errorf(logger.CodeTrnDial, "stage[network] failed", err)
		return nil, errs.Retry(errs.WithCode(logger.CodeTrnDial, err))
	}

	// Stage 2: TLS
	if t.cfg.Transport.TLS {
		sub := t.log.For(logger.CompTLS).Event("transport.tls.handshake")
		done := sub.Slow("tls.handshake", 5*time.Second)
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         t.cfg.SNIHost(),
			InsecureSkipVerify: true,
		})
		hctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := tlsConn.HandshakeContext(hctx); err != nil {
			done(err)
			sub.Errorf(logger.CodeTrnTLSAuth, "tls handshake failed", err,
				"sni", t.cfg.SNIHost())
			_ = conn.Close()
			return nil, errs.Retry(errs.WithCode(logger.CodeTrnTLSAuth, err))
		}
		done(nil)
		sub.Info("tls handshake ok", "sni", t.cfg.SNIHost())
		conn = tlsConn
	}

	// Stage 3: payload
	if t.cfg.Payload.Enable {
		pl := t.log.For(logger.CompPayload).Event("transport.payload.inject")
		var perr error
		conn, perr = applyPayload(conn, t.cfg.Payload, t.cfg.PayloadHost(), t.cfg.SSH.Port, pl)
		if perr != nil {
			_ = conn.Close()
			return nil, errs.Retry(perr)
		}
	}

	lg.Debug("transport pipeline complete")
	return conn, nil
}