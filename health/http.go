package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/youruser/gosshtunnel/internal/logger"
)

type Server struct {
	Addr       string
	SnapshotFn func() Snapshot
	log        *logger.Logger
	srv        *http.Server
}

func NewServer(addr string, fn func() Snapshot, log *logger.Logger) *Server {
	return &Server{
		Addr: addr, SnapshotFn: fn,
		log: log.For(logger.CompHealth),
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)

	s.srv = &http.Server{
		Addr:         s.Addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
	}()

	s.log.Info("health endpoint listening", "addr", s.Addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	snap := s.SnapshotFn()
	w.Header().Set("Content-Type", "application/json")
	if snap.State != StateHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(snap)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	snap := s.SnapshotFn()
	if snap.State.Healthy() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("not ready: " + snap.State.String()))
}