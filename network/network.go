package network

import (
	"fmt"

	"github.com/youruser/gosshtunnel/config"
	"github.com/youruser/gosshtunnel/internal/logger"
)

func NewDialer(cfg *config.Config, log *logger.Logger) (Dialer, error) {
	target := cfg.DialTarget()
	switch cfg.Network.Type {
	case "tcp", "":
		return &TCPDialer{Address: target, Log: log}, nil
	case "ws":
		path := cfg.Transport.Path
		if path == "" {
			path = "/"
		}
		return NewWSDialer(target, path, cfg.Transport.TLS, cfg.SNIHost(), log), nil
	default:
		return nil, fmt.Errorf("network.type tidak didukung: %s", cfg.Network.Type)
	}
}