package clashapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/youruser/gosshtunnel/internal/clashconfig"
	"github.com/youruser/gosshtunnel/internal/logger"
	"github.com/youruser/gosshtunnel/internal/rules"
)

type WorkerRef struct {
	Name     string
	Addr     string
	Healthy  func() bool
	Latency  func() time.Duration
	Snapshot func() interface{}
}

type Options struct {
	Listen      string
	Secret      string
	Version     string
	CORSOrigins string
	ExternalUI  string

	Workers     []WorkerRef
	Traffic     *TrafficTracker
	Connections *ConnectionTracker
	Logs        *LogBroadcaster
	Logger      *logger.Logger

	SelectorName    string
	ActiveWorker    func() string
	SetActiveWorker func(name string) error

	RuleEngine  *rules.Engine
	ClashConfig *clashconfig.ClashConfig
}

type Server struct {
	opt      Options
	log      *logger.Logger
	srv      *http.Server
	upgrader websocket.Upgrader
}

func NewServer(opt Options) *Server {
	if opt.SelectorName == "" {
		opt.SelectorName = "WORKERS"
	}
	if opt.Version == "" {
		opt.Version = "1.0.0"
	}
	return &Server{
		opt: opt,
		log: opt.Logger.For(logger.CompClashAPI),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := corsMiddleware(s.opt.CORSOrigins)(authMiddleware(s.opt.Secret, mux))

	s.srv = &http.Server{
		Addr:         s.opt.Listen,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
	}()

	s.log.Info("clash api listening",
		"addr", s.opt.Listen, "secret_set", s.opt.Secret != "")
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) findWorker(name string) *WorkerRef {
	for i := range s.opt.Workers {
		if s.opt.Workers[i].Name == name {
			return &s.opt.Workers[i]
		}
	}
	return nil
}