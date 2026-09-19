package transport

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/youruser/gosshtunnel/config"
)

func DialThroughProxy(ctx context.Context, p config.ProxyConfig, target string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port)))
	if err != nil {
		return nil, err
	}
	switch p.Type {
	case "http", "":
		return httpConnect(conn, target, p)
	default:
		conn.Close()
		return nil, fmt.Errorf("proxy.type tidak didukung: %s", p.Type)
	}
}

func httpConnect(conn net.Conn, target string, p config.ProxyConfig) (net.Conn, error) {
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "http", Host: target},
		Host:   target,
		Header: http.Header{},
	}
	if p.Username != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(p.Username + ":" + p.Password))
		req.Header.Set("Proxy-Authorization", "Basic "+auth)
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT gagal: %s", resp.Status)
	}
	_ = conn.SetDeadline(time.Time{})
	if br.Buffered() > 0 {
		return &bufferedConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}