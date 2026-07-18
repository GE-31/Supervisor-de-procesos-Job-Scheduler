package health

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/worker"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type Provider interface{ Snapshot() worker.Snapshot }
type CrashFunc func(int)
type Server struct {
	http     *http.Server
	provider Provider
	logger   *slog.Logger
	crash    CrashFunc
}

func New(addr string, provider Provider, logger *slog.Logger, crash CrashFunc) *Server {
	if crash == nil {
		crash = os.Exit
	}
	s := &Server{provider: provider, logger: logger, crash: crash}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/metrics", s.metrics)
	mux.HandleFunc("/demo/crash", s.demoCrash)
	s.http = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	return s
}
func (s *Server) Start() error {
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	snap := s.provider.Snapshot()
	status := http.StatusOK
	if !snap.OrdersAPIConnected {
		status = http.StatusServiceUnavailable
	}
	write(w, status, map[string]any{"status": map[bool]string{true: "ok", false: "degraded"}[snap.OrdersAPIConnected], "service": "orders-worker", "worker_state": snap.Status, "pid": snap.PID, "uptime_seconds": snap.UptimeSeconds, "orders_api_connected": snap.OrdersAPIConnected, "active_jobs": snap.ActiveJobs, "processed_orders": snap.ProcessedOrders, "failed_orders": snap.FailedOrders, "last_error": snap.LastError})
}
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	v := s.provider.Snapshot()
	write(w, http.StatusOK, map[string]any{"processed_orders": v.ProcessedOrders, "failed_orders": v.FailedOrders, "active_jobs": v.ActiveJobs, "api_failures": v.APIFailures, "reconnections": v.Reconnections})
}
func (s *Server) demoCrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	v := s.provider.Snapshot()
	s.logger.Error("caída de demostración solicitada", "service", "orders-worker", "pid", v.PID)
	write(w, http.StatusInternalServerError, map[string]any{"message": "Caída de demostración iniciada", "pid": v.PID})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() { time.Sleep(500 * time.Millisecond); s.crash(1) }()
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func method(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	write(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": "Método HTTP no permitido"})
}
func (s *Server) Handler() http.Handler { return s.http.Handler }
