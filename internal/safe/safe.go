package safe

import (
	"runtime/debug"

	"github.com/youruser/gosshtunnel/internal/logger"
)

func Go(log *logger.Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Slog().Error("goroutine panic",
					"goroutine", name,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}