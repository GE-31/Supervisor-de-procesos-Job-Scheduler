package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

func dialTestServer(t *testing.T, server *httptest.Server) *coderws.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func readEvent(t *testing.T, conn *coderws.Conn) Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("leer mensaje: %v", err)
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decodificar evento %q: %v", data, err)
	}
	return event
}

func TestHandlerSendsInitialSnapshot(t *testing.T) {
	lister := &fakeLister{}
	lister.set([]supervisor.Snapshot{{Name: "demo", State: supervisor.StateRunning}})
	hub := NewHub(silentLogger())
	defer hub.Close()
	hub.SetLister(lister)

	server := httptest.NewServer(Handler(hub, silentLogger()))
	defer server.Close()

	conn := dialTestServer(t, server)
	defer conn.Close(coderws.StatusNormalClosure, "")

	event := readEvent(t, conn)
	if event.Type != "jobs.snapshot" {
		t.Fatalf("Type = %q, se esperaba jobs.snapshot", event.Type)
	}
	if len(event.Jobs) != 1 || event.Jobs[0].Name != "demo" {
		t.Fatalf("Jobs = %+v", event.Jobs)
	}
}

func TestHandlerDeliversJobChangeEvent(t *testing.T) {
	lister := &fakeLister{}
	lister.set([]supervisor.Snapshot{{Name: "demo", State: supervisor.StateRunning}})
	hub := NewHub(silentLogger())
	defer hub.Close()
	hub.SetLister(lister)

	server := httptest.NewServer(Handler(hub, silentLogger()))
	defer server.Close()

	conn := dialTestServer(t, server)
	defer conn.Close(coderws.StatusNormalClosure, "")
	readEvent(t, conn) // descarta el snapshot inicial

	hub.PublishJobChange(supervisor.Snapshot{Name: "demo", State: supervisor.StateStopping})
	event := readEvent(t, conn)
	if event.Type != "job.stopping" || event.Job == nil || event.Job.Name != "demo" {
		t.Fatalf("event = %+v", event)
	}
}

func TestHandlerMultipleClientsReceiveSameEvent(t *testing.T) {
	lister := &fakeLister{}
	lister.set([]supervisor.Snapshot{{Name: "demo", State: supervisor.StateRunning}})
	hub := NewHub(silentLogger())
	defer hub.Close()
	hub.SetLister(lister)

	server := httptest.NewServer(Handler(hub, silentLogger()))
	defer server.Close()

	connA := dialTestServer(t, server)
	defer connA.Close(coderws.StatusNormalClosure, "")
	connB := dialTestServer(t, server)
	defer connB.Close(coderws.StatusNormalClosure, "")
	readEvent(t, connA)
	readEvent(t, connB)

	hub.PublishJobChange(supervisor.Snapshot{Name: "demo", State: supervisor.StateFailed})
	for _, conn := range []*coderws.Conn{connA, connB} {
		if event := readEvent(t, conn); event.Type != "job.failed" {
			t.Errorf("Type = %q", event.Type)
		}
	}
}

func TestHandlerDisconnectCleansUpHub(t *testing.T) {
	hub := NewHub(silentLogger())
	defer hub.Close()
	hub.SetLister(&fakeLister{})

	server := httptest.NewServer(Handler(hub, silentLogger()))
	defer server.Close()

	conn := dialTestServer(t, server)
	readEvent(t, conn) // snapshot inicial: confirma que ya está registrado

	deadline := time.Now().Add(2 * time.Second)
	for hub.count() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.count() != 1 {
		t.Fatalf("count() = %d, se esperaba 1 antes de desconectar", hub.count())
	}

	conn.Close(coderws.StatusNormalClosure, "cierre de prueba")

	deadline = time.Now().Add(2 * time.Second)
	for hub.count() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.count() != 0 {
		t.Fatalf("count() = %d, se esperaba 0 tras desconectar", hub.count())
	}
}

func TestHandlerRejectsNonGet(t *testing.T) {
	hub := NewHub(silentLogger())
	defer hub.Close()
	server := httptest.NewServer(Handler(hub, silentLogger()))
	defer server.Close()

	resp, err := http.Post(server.URL, "application/json", nil)
	if err != nil {
		t.Fatalf("POST /ws: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, se esperaba 405", resp.StatusCode)
	}
}

func TestHandlerClosesConnectionsOnHubClose(t *testing.T) {
	hub := NewHub(silentLogger())
	hub.SetLister(&fakeLister{})
	server := httptest.NewServer(Handler(hub, silentLogger()))
	defer server.Close()

	conn := dialTestServer(t, server)
	defer conn.Close(coderws.StatusNormalClosure, "")
	readEvent(t, conn)

	hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("se esperaba que la conexión se cerrara al cerrar el hub")
	}
}
