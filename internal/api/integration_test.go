package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/api/websocket"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

// Estas pruebas usan el supervisor real (no fakeController) porque
// necesitan sus transiciones de estado reales: cuándo devuelve
// ErrInvalidState (409) y cuándo publica cada evento.

type discardOutputStore struct{}

func (discardOutputStore) Writer(string, string) (io.WriteCloser, error) {
	return nopWriteCloser{Writer: io.Discard}, nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

const loopScript = "trap 'exit 0' TERM; while :; do sleep 1; done"

func newLoopSupervisor(t *testing.T, name string, publish ...supervisor.Publisher) *supervisor.Supervisor {
	t.Helper()
	specs := []supervisor.JobSpec{{Name: name, Command: "sh", Args: []string{"-c", loopScript}, Restart: "never", WorkDir: ".", MaxRetries: 0}}
	sup, err := supervisor.New(specs, supervisor.Backoff{Base: 5 * time.Millisecond, Factor: 2, Max: 20 * time.Millisecond}, 200*time.Millisecond, discardOutputStore{}, publish...)
	if err != nil {
		t.Fatalf("crear supervisor: %v", err)
	}
	return sup
}

func waitForState(t *testing.T, sup *supervisor.Supervisor, name string, want supervisor.State, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := sup.Job(name)
		if err != nil {
			t.Fatal(err)
		}
		if snap.State == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	snap, _ := sup.Job(name)
	t.Fatalf("estado final %q, se esperaba %q", snap.State, want)
}

func TestIntegrationStartAlreadyRunningReturns409(t *testing.T) {
	t.Chdir(repoRoot(t))
	sup := newLoopSupervisor(t, "loop")
	srv, err := NewServer(":0", sup, fakeLogs{}, log.New(io.Discard, "", 0), nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := srv.routes()

	if err := sup.StartJob("loop"); err != nil {
		t.Fatal(err)
	}
	waitForState(t, sup, "loop", supervisor.StateRunning, time.Second)
	defer sup.StopJob("loop")

	w := request(t, handler, "POST", "/api/jobs/loop/start")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, se esperaba 409 (Conflict)", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] != "invalid_state" {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestIntegrationStopUnknownJobReturns404(t *testing.T) {
	t.Chdir(repoRoot(t))
	sup := newLoopSupervisor(t, "loop")
	srv, err := NewServer(":0", sup, fakeLogs{}, log.New(io.Discard, "", 0), nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	w := request(t, srv.routes(), "POST", "/api/jobs/no-existe/stop")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, se esperaba 404", w.Code)
	}
}

func readWSEvent(t *testing.T, conn *coderws.Conn) websocket.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("leer evento websocket: %v", err)
	}
	var event websocket.Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decodificar evento %q: %v", data, err)
	}
	return event
}

// TestIntegrationActionsProduceWebSocketEvents ejercita la pila completa:
// arranca un job real a través del supervisor, lo detiene con una solicitud
// HTTP real (POST /api/jobs/{name}/stop, tal como lo haría el navegador), y
// comprueba que cada transición de estado llega por /ws sin necesidad de
// recargar ninguna página.
func TestIntegrationActionsProduceWebSocketEvents(t *testing.T) {
	t.Chdir(repoRoot(t))
	logger := log.New(io.Discard, "", 0)
	hub := websocket.NewHub(logger)
	defer hub.Close()

	sup := newLoopSupervisor(t, "loop", hub.PublishJobChange)
	hub.SetLister(sup)

	srv, err := NewServer(":0", sup, fakeLogs{}, logger, hub)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	testServer := httptest.NewServer(srv.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws"
	dialCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(dialCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial /ws: %v", err)
	}
	defer conn.Close(coderws.StatusNormalClosure, "")

	if event := readWSEvent(t, conn); event.Type != "jobs.snapshot" {
		t.Fatalf("primer mensaje = %q, se esperaba jobs.snapshot", event.Type)
	}

	if err := sup.StartJob("loop"); err != nil {
		t.Fatal(err)
	}
	if event := readWSEvent(t, conn); event.Type != "job.starting" {
		t.Fatalf("evento = %q, se esperaba job.starting", event.Type)
	}
	if event := readWSEvent(t, conn); event.Type != "job.running" {
		t.Fatalf("evento = %q, se esperaba job.running", event.Type)
	}

	resp, err := http.Post(testServer.URL+"/api/jobs/loop/stop", "application/json", nil)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, se esperaba 202 (Accepted)", resp.StatusCode)
	}

	if event := readWSEvent(t, conn); event.Type != "job.stopping" {
		t.Fatalf("evento = %q, se esperaba job.stopping", event.Type)
	}
	if event := readWSEvent(t, conn); event.Type != "job.stopped" {
		t.Fatalf("evento = %q, se esperaba job.stopped", event.Type)
	}
}
