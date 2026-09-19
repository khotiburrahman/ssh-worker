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

// RenderPayload mengganti token template HTTP injector.
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

// applyPayload mengikuti flow HTTP Injector:
//
//  1. Kirim semua bagian SEBELUM [split] sekaligus (request pertama + kedua)
//  2. Baca semua HTTP response yang datang (drain buffer)
//  3. Kirim semua bagian SETELAH [split] (padding)
//  4. Kembalikan conn — siap untuk SSH handshake
//
// Tidak melakukan validasi expect yang ketat, karena server proxy
// sering kasih response berbeda (301, 403, 200) tapi tetap meneruskan
// koneksi ke SSH server.
func applyPayload(conn net.Conn, cfg config.PayloadConfig, host string, port int, lg *logger.Logger) (net.Conn, error) {
	if !cfg.Enable || cfg.Request == "" {
		return conn, nil
	}

	parts := strings.Split(cfg.Request, "[split]")
	if len(parts) == 0 {
		return conn, nil
	}

	// Render semua bagian
	for i := range parts {
		parts[i] = RenderPayload(parts[i], host, port)
	}

	head := parts[0]                             // request #1 + #2
	tail := ""                                   // padding setelah [split]
	if len(parts) > 1 {
		tail = strings.Join(parts[1:], "")
	}

	lg.Debug("payload prepare",
		"head_bytes", len(head),
		"tail_bytes", len(tail),
		"host", host,
	)

	// -------- STEP 1: Kirim HEAD --------
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write([]byte(head)); err != nil {
		lg.Errorf(logger.CodeTrnPayloadWr, "payload head write failed", err)
		return conn, errs.WithCode(logger.CodeTrnPayloadWr, err)
	}
	_ = conn.SetWriteDeadline(time.Time{})
	lg.Debug("payload head sent", "bytes", len(head))

	// -------- STEP 2: Baca semua response (drain) --------
	br := bufio.NewReader(conn)
	statuses := drainResponses(conn, br, lg, 3*time.Second)
	lg.Info("payload responses drained",
		"count", len(statuses),
		"statuses", statuses,
	)

	// Validasi toleran: kalau ada expect dan ada minimal 1 status,
	// cek apakah ada yang match. Kalau tidak match — tetap lanjut
	// (server mungkin balas berbeda tapi tetap tunnel).
	if len(cfg.Expect) > 0 && len(statuses) > 0 {
		matched := false
		for _, s := range statuses {
			if matchExpect(s, cfg.Expect) {
				matched = true
				break
			}
		}
		if !matched {
			lg.Warn("payload expect tidak match, tetap lanjut (tolerant mode)",
				"code", logger.CodeTrnPayloadEx,
				"got", statuses,
				"expect", cfg.Expect,
			)
		}
	}

	// -------- STEP 3: Kirim TAIL --------
	if tail != "" {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write([]byte(tail)); err != nil {
			lg.Errorf(logger.CodeTrnPayloadWr, "payload tail write failed", err)
			return conn, errs.WithCode(logger.CodeTrnPayloadWr, err)
		}
		_ = conn.SetWriteDeadline(time.Time{})
		lg.Debug("payload tail sent", "bytes", len(tail))
	}

	// Beri jeda kecil agar proxy settle
	time.Sleep(100 * time.Millisecond)

	// Bungkus sisa buffer (kalau ada)
	if br.Buffered() > 0 {
		lg.Debug("payload leftover in buffer", "bytes", br.Buffered())
		return &bufferedConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

// drainResponses membaca semua HTTP response yang datang sampai timeout.
// Return daftar status line (misal "HTTP/1.1 301 Moved Permanently").
func drainResponses(conn net.Conn, br *bufio.Reader, lg *logger.Logger, timeout time.Duration) []string {
	var statuses []string
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(remaining))

		status, err := readHTTPHead(br)
		if err != nil {
			if isTimeout(err) || errors.Is(err, io.EOF) {
				break
			}
			lg.Debug("drain response error", "err", err)
			break
		}
		if status != "" {
			statuses = append(statuses, strings.TrimSpace(status))
		}

		// Kalau sudah dapat 101 Switching Protocols, tidak perlu baca lagi
		if strings.Contains(status, "101") {
			break
		}
	}

	_ = conn.SetReadDeadline(time.Time{})
	return statuses
}

// readHTTPHead membaca status line + semua header sampai CRLFCRLF.
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