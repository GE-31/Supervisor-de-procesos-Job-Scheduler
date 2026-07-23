package websocket

import (
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

func silentLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// fakeLister implementa JobLister con una lista fija, protegida por mutex
// porque las pruebas de concurrencia la leen y escriben desde goroutines
// distintas.
type fakeLister struct {
	mu   sync.Mutex
	jobs []supervisor.Snapshot
}

func (f *fakeLister) ListJobs() []supervisor.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]supervisor.Snapshot(nil), f.jobs...)
}
func (f *fakeLister) set(jobs []supervisor.Snapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = jobs
}

// testClient crea un Client sin conexión real: válido para probar la
// lógica del hub (registro, broadcast, desconexión) porque evict() es
// seguro ante conn nil.
func testClient() *Client {
	return &Client{send: make(chan Event, clientSendBuffer)}
}

func recvEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("no se recibió el evento esperado a tiempo")
		return Event{}
	}
}

func expectNoEvent(t *testing.T, ch <-chan Event) {
	t.Helper()
	select {
	case event := <-ch:
		t.Fatalf("no se esperaba ningún evento, llegó: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHubBroadcastReachesAllClients(t *testing.T) {
	hub := NewHub(silentLogger())
	defer hub.Close()
	a, b := testClient(), testClient()
	if !hub.Register(a) || !hub.Register(b) {
		t.Fatal("Register() falló")
	}
	hub.Broadcast(Event{Type: "job.running", Timestamp: time.Now()})
	for _, c := range []*Client{a, b} {
		if event := recvEvent(t, c.send); event.Type != "job.running" {
			t.Errorf("event.Type = %q", event.Type)
		}
	}
}

func TestHubUnregisterStopsDelivery(t *testing.T) {
	hub := NewHub(silentLogger())
	defer hub.Close()
	c := testClient()
	hub.Register(c)
	hub.Unregister(c)
	for hub.count() != 0 {
		time.Sleep(time.Millisecond)
	}
	hub.Broadcast(Event{Type: "job.running", Timestamp: time.Now()})
	expectNoEvent(t, c.send)
}

func TestHubSlowClientIsEvicted(t *testing.T) {
	hub := NewHub(silentLogger())
	defer hub.Close()
	slow := testClient()
	hub.Register(slow)

	// Satura el búfer del cliente sin drenarlo: el próximo Broadcast lo
	// encuentra lleno y el hub lo desconecta en vez de bloquear al resto.
	for i := 0; i < clientSendBuffer+1; i++ {
		hub.Broadcast(Event{Type: "job.running", Timestamp: time.Now()})
	}
	for hub.count() != 0 {
		time.Sleep(time.Millisecond)
	}

	// Vaciamos lo que sí llegó a encolarse y confirmamos que ya no llega
	// nada nuevo: si siguiera registrado, este Broadcast sí se entregaría
	// porque el búfer ahora está vacío.
	for len(slow.send) > 0 {
		<-slow.send
	}
	hub.Broadcast(Event{Type: "job.running", Timestamp: time.Now()})
	expectNoEvent(t, slow.send)
}

func TestHubSnapshotIncludesJobsAndSummary(t *testing.T) {
	lister := &fakeLister{}
	lister.set([]supervisor.Snapshot{
		{Name: "a", State: supervisor.StateRunning},
		{Name: "b", State: supervisor.StateStopped},
		{Name: "c", State: supervisor.StateFailed},
	})
	hub := NewHub(silentLogger())
	defer hub.Close()
	hub.SetLister(lister)

	snap := hub.Snapshot()
	if snap.Type != "jobs.snapshot" {
		t.Fatalf("Type = %q", snap.Type)
	}
	if len(snap.Jobs) != 3 {
		t.Fatalf("Jobs = %d, se esperaban 3", len(snap.Jobs))
	}
	if snap.Summary == nil || snap.Summary.Running != 1 || snap.Summary.Stopped != 1 || snap.Summary.Failed != 1 {
		t.Fatalf("Summary = %+v", snap.Summary)
	}
}

func TestHubPublishJobChangeEmitsTypedEvent(t *testing.T) {
	lister := &fakeLister{}
	lister.set([]supervisor.Snapshot{{Name: "a", State: supervisor.StateRunning}})
	hub := NewHub(silentLogger())
	defer hub.Close()
	hub.SetLister(lister)

	c := testClient()
	hub.Register(c)

	hub.PublishJobChange(supervisor.Snapshot{Name: "a", State: supervisor.StateBackoff, Retries: 2})
	event := recvEvent(t, c.send)
	if event.Type != "job.backoff" {
		t.Errorf("Type = %q, se esperaba job.backoff", event.Type)
	}
	if event.Job == nil || event.Job.Name != "a" {
		t.Fatalf("Job = %+v", event.Job)
	}
	if event.Summary == nil {
		t.Fatal("se esperaba un resumen recalculado en el mismo evento")
	}
}

func TestHubEventTypesCoverAllRealStates(t *testing.T) {
	cases := map[supervisor.State]string{
		supervisor.StateStarting: "job.starting",
		supervisor.StateRunning:  "job.running",
		supervisor.StateBackoff:  "job.backoff",
		supervisor.StateStopping: "job.stopping",
		supervisor.StateStopped:  "job.stopped",
		supervisor.StateFailed:   "job.failed",
	}
	for state, want := range cases {
		if got := jobEventType(state); got != want {
			t.Errorf("jobEventType(%q) = %q, se esperaba %q", state, got, want)
		}
	}
}

func TestHubCloseIsIdempotentAndStopsTraffic(t *testing.T) {
	hub := NewHub(silentLogger())
	c := testClient()
	hub.Register(c)
	hub.Close()
	hub.Close() // no debe bloquear ni paniquear al llamarse dos veces.

	if hub.Register(testClient()) {
		t.Fatal("Register() no debería aceptar clientes después de Close()")
	}
	// Broadcast tras Close no debe bloquear ni paniquear.
	done := make(chan struct{})
	go func() { hub.Broadcast(Event{Type: "job.running"}); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast() se quedó bloqueado después de Close()")
	}
}

func TestHubConcurrentRegisterBroadcastUnregister(t *testing.T) {
	hub := NewHub(silentLogger())
	defer hub.Close()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// El búfer (16) cubre de sobra el único evento que cada
			// goroutine dispara, así que no hace falta drenar c.send: el
			// canal nunca se cierra (ver evict()) y "range" sobre él
			// bloquearía para siempre.
			c := testClient()
			hub.Register(c)
			hub.Broadcast(Event{Type: "job.running", Timestamp: time.Now()})
			hub.Unregister(c)
		}()
	}
	wg.Wait()
}
