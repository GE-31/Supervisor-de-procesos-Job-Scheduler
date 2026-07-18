package health

import (
	"encoding/json"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/worker"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type provider struct{ snapshot worker.Snapshot }

func (p provider) Snapshot() worker.Snapshot { return p.snapshot }
func TestHealthMetricsAndMethods(t *testing.T) {
	p := provider{worker.Snapshot{Status: worker.StatusRunning, OrdersAPIConnected: true, ProcessedOrders: 2}}
	s := New("", p, slog.New(slog.NewTextHandler(io.Discard, nil)), func(int) {})
	for _, path := range []string{"/health", "/metrics"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s=%d", path, w.Code)
		}
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil {
			t.Fatal("JSON inválido")
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("method=%d", w.Code)
	}
}
func TestDegradedAndCrash(t *testing.T) {
	called := make(chan int, 1)
	s := New("", provider{worker.Snapshot{Status: worker.StatusDegraded}}, slog.New(slog.NewTextHandler(io.Discard, nil)), func(code int) { called <- code })
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("degraded=%d", w.Code)
	}
	r = httptest.NewRequest(http.MethodPost, "/demo/crash", nil)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("crash no llamado")
	}
}
