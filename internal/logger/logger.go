package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"
	"time"
)

type Component string

const (
	CompApp        Component = "app"
	CompConfig     Component = "config"
	CompNetwork    Component = "network"
	CompTransport  Component = "transport"
	CompPayload    Component = "payload"
	CompTLS        Component = "tls"
	CompProxy      Component = "proxy"
	CompQLoad      Component = "qload"
	CompSSH        Component = "ssh"
	CompSOCKS5     Component = "socks5"
	CompWorker     Component = "worker"
	CompHealth     Component = "health"
	CompRules      Component = "rules"
	CompClashCfg   Component = "clashconfig"
	CompClashAPI   Component = "clashapi"
	CompGeodata    Component = "geodata"
)

type Logger struct {
	inner     *slog.Logger
	component Component
}

func New(level string) *Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})
	return &Logger{
		inner:     slog.New(h).With("component", string(CompApp)),
		component: CompApp,
	}
}

func (l *Logger) For(c Component) *Logger {
	return &Logger{
		inner:     l.inner.With("component", string(c)),
		component: c,
	}
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		inner:     l.inner.With(args...),
		component: l.component,
	}
}

func (l *Logger) Worker(idx int, listen string) *Logger {
	return l.With("worker", idx, "listen", listen)
}

func (l *Logger) Slog() *slog.Logger { return l.inner }

func (l *Logger) Event(name string) *Logger {
	return &Logger{
		inner:     l.inner.With("event", name),
		component: l.component,
	}
}

func (l *Logger) Code(code string) *Logger {
	return &Logger{
		inner:     l.inner.With("code", code),
		component: l.component,
	}
}

func (l *Logger) Debug(msg string, args ...any) { l.inner.Debug(msg, args...) }
func (l *Logger) Info(msg string, args ...any)  { l.inner.Info(msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.inner.Warn(msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.inner.Error(msg, args...) }

func (l *Logger) Errorf(code, msg string, err error, args ...any) {
	all := append([]any{"code", code, "err", err}, args...)
	l.inner.Error(msg, all...)
}

// Slow memonitor durasi operasi; warn jika > threshold.
func (l *Logger) Slow(op string, threshold time.Duration) func(err error) {
	start := time.Now()
	return func(err error) {
		d := time.Since(start)
		switch {
		case err != nil:
			l.inner.Error("op failed", "op", op, "dur", d.String(), "err", err)
		case d >= threshold:
			l.inner.Warn("op slow", "op", op, "dur", d.String(), "threshold", threshold.String())
		default:
			l.inner.Debug("op ok", "op", op, "dur", d.String())
		}
	}
}

func (l *Logger) PanicRecover(name string) {
	if r := recover(); r != nil {
		l.inner.Error("goroutine panic",
			"code", CodeWrkPanic,
			"goroutine", name,
			"panic", r,
			"stack", string(debug.Stack()),
		)
	}
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	if v := ctx.Value(traceKey{}); v != nil {
		return l.With("trace_id", v)
	}
	return l
}

type traceKey struct{}