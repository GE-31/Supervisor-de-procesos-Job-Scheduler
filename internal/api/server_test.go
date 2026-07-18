package api

import (
	"bytes"
	"encoding/json"
	joblog "github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/logging"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeController struct {
	job    supervisor.Snapshot
	action string
}

func (f *fakeController) ListJobs() []supervisor.Snapshot { return []supervisor.Snapshot{f.job} }
func (f *fakeController) Job(name string) (supervisor.Snapshot, error) {
	if name != f.job.Name {
		return supervisor.Snapshot{}, supervisor.ErrNotFound
	}
	return f.job, nil
}
func (f *fakeController) StartJob(name string) error {
	if _, err := f.Job(name); err != nil {
		return err
	}
	f.action = "start"
	return nil
}
func (f *fakeController) StopJob(name string) error {
	if _, err := f.Job(name); err != nil {
		return err
	}
	f.action = "stop"
	return nil
}
func (f *fakeController) RestartJob(name string) error {
	if _, err := f.Job(name); err != nil {
		return err
	}
	f.action = "restart"
	return nil
}

type fakeLogs struct{}

func (fakeLogs) Read(string, int) ([]joblog.Entry, error) { return []joblog.Entry{}, nil }
func testHandler() (*fakeController, http.Handler) {
	c := &fakeController{job: supervisor.Snapshot{Name: "demo", Command: "echo", State: supervisor.StateRunning, PID: 42, RestartPolicy: "never", StartedAt: time.Now()}}
	s := &Server{controller: c, logs: fakeLogs{}, logger: log.New(io.Discard, "", 0)}
	return c, s.routes()
}
func request(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(nil))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestHealthAndJSON(t *testing.T) {
	_, h := testHandler()
	w := request(t, h, "GET", "/api/health")
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type=%s", got)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["status"] != "ok" {
		t.Fatalf("body=%s", w.Body.String())
	}
}
func TestSummaryAndJobs(t *testing.T) {
	_, h := testHandler()
	for _, path := range []string{"/api/summary", "/api/jobs", "/api/jobs/demo"} {
		w := request(t, h, "GET", path)
		if w.Code != 200 {
			t.Errorf("%s status=%d", path, w.Code)
		}
	}
}
func TestNotFoundAndMethod(t *testing.T) {
	_, h := testHandler()
	if w := request(t, h, "GET", "/api/jobs/missing"); w.Code != 404 {
		t.Fatalf("not found=%d", w.Code)
	}
	if w := request(t, h, "PUT", "/api/jobs"); w.Code != 405 {
		t.Fatalf("method=%d", w.Code)
	}
}
func TestActions(t *testing.T) {
	for _, action := range []string{"start", "stop", "restart"} {
		c, h := testHandler()
		w := request(t, h, "POST", "/api/jobs/demo/"+action)
		if w.Code != http.StatusAccepted {
			t.Errorf("%s status=%d", action, w.Code)
		}
		if c.action != action {
			t.Errorf("action=%q", c.action)
		}
	}
}
func TestLogsAndInvalidLimit(t *testing.T) {
	_, h := testHandler()
	if w := request(t, h, "GET", "/api/jobs/demo/logs?limit=50"); w.Code != 200 {
		t.Fatalf("logs=%d", w.Code)
	}
	if w := request(t, h, "GET", "/api/jobs/demo/logs?limit=5000"); w.Code != 400 {
		t.Fatalf("limit=%d", w.Code)
	}
}
