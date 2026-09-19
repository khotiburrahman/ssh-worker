package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/youruser/gosshtunnel/internal/logger"
)

type WSDialer struct {
	Address    string
	Path       string
	TLS        bool
	ServerName string
	Headers    http.Header
	Log        *logger.Logger
}

func NewWSDialer(addr, path string, useTLS bool, sni string, log *logger.Logger) *WSDialer {
	return &WSDialer{
		Address: addr, Path: path, TLS: useTLS, ServerName: sni,
		Headers: http.Header{},
		Log:     log.For(logger.CompNetwork),
	}
}

func (d *WSDialer) DialContext(ctx context.Context) (net.Conn, error) {
	lg := d.Log.Event("net.ws.dial")
	done := lg.Slow("ws.dial", 5*time.Second)

	scheme := "ws"
	if d.TLS {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: d.Address, Path: d.Path}

	dl := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	if d.TLS {
		dl.TLSClientConfig = &tls.Config{
			ServerName:         d.ServerName,
			InsecureSkipVerify: true,
		}
	}

	ws, resp, err := dl.DialContext(ctx, u.String(), d.Headers)
	done(err)
	if err != nil {
		if resp != nil {
			lg.Errorf(logger.CodeNetWSUpgrade, "ws upgrade failed", err,
				"url", u.String(), "status", resp.StatusCode)
		} else {
			lg.Errorf(logger.CodeNetWSUpgrade, "ws dial failed", err, "url", u.String())
		}
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	lg.Info("ws connected", "url", u.String())
	return newWSConn(ws), nil
}