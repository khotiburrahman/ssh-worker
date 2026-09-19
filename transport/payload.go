package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/youruser/gosshtunnel/config"
	"github.com/youruser/gosshtunnel/internal/errs"
	"github.com/youruser/gosshtunnel/internal/logger"
)

func RenderPayload(template, host string, port int) string {
	r := strings.NewReplacer(
		"[crlf]", "\r\n", "[CRLF]", "\r\n",
		"[lf]", "\n", "[LF]", "\n",
		"[cr]", "\r", "[CR]", "\r",
		"[tab]", "\t", "[TAB]", "\t",
		"[host]", host, "[Host]", host, "[HOST]", host,
		"[port]", fmt.Sprintf("%d", port),
		"[PORT]", fmt.Sprintf("%d", port),
	)
	return r.Replace(template)
}

func applyPayload(conn net.Conn, cfg config.PayloadConfig, host string, port int, lg *logger.Logger) (net.Conn, error) {
	if !cfg.Enable || cfg.Request == "" {
		return conn, nil
	}
	done := lg.Slow("payload.inject", 5*time.Second)

	parts := strings.Split(cfg.Request, "[split]")
	for i := range parts {
		parts[i] = RenderPayload(parts[i], host, port)
	}

	if _, err := conn.Write([]byte(parts[0])); err != nil {
		done(err)
		lg.Errorf(logger.CodeTrnPayloadWr, "payload write failed", err,
			"bytes", len(parts[0]), "host", host)
		return nil, errs.WithCode(logger.CodeTrnPayloadWr, err)
	}
	lg.Debug("payload sent", "bytes", len(parts[0]), "parts", len(parts))

	br := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	statusLine, err := readHTTPHead(br)
	_ = conn.SetReadDeadline(time.Time{})

	if err != nil {
		if !errors.Is(err, io.EOF) && !isTimeout(err) {
			done(err)
			lg.Errorf(logger.CodeTrnPayloadRd, "payload read failed", err)
			return nil, errs.WithCode(logger.CodeTrnPayloadRd, err)
		}
		lg.Debug("payload read timeout/EOF — dianggap OK (tunnel mode)")
	} else {
		status := strings.TrimSpace(statusLine)
		lg.Info("payload response", "status", status, "expect", cfg.Expect)
		if len(cfg.Expect) > 0 && !matchExpect(statusLine, cfg.Expect) {
			err := fmt.Errorf("status %q tidak match expect %v", status, cfg.Expect)
			done(err)
			lg.Errorf(logger.CodeTrnPayloadEx, "payload expect mismatch", err)
			return nil, errs.WithCode(logger.CodeTrnPayloadEx, err)
		}
	}

	for _, p := range parts[1:] {
		if _, err := conn.Write([]byte(p)); err != nil {
			done(err)
			lg.Errorf(logger.CodeTrnPayloadWr, "payload tail write failed", err)
			return nil, errs.WithCode(logger.CodeTrnPayloadWr, err)
		}
	}

	done(nil)
	if br.Buffered() > 0 {
		return &bufferedConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

func readHTTPHead(br *bufio.Reader) (string, error) {
	status, err := br.ReadString('\n')
	if err != nil {
		return "", err
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return status, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return status, nil
}

func matchExpect(statusLine string, expect []string) bool {
	for _, e := range expect {
		if strings.Contains(statusLine, e) {
			return true
		}
	}
	return false
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}