package worker

import (
	"context"
	"errors"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/client"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeAPI struct {
	mu     sync.Mutex
	calls  int
	fail   int
	orders []client.Order
}

func (a *fakeAPI) Health(context.Context) error { return nil }
func (a *fakeAPI) ListOrders(context.Context) ([]client.Order, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if a.calls <= a.fail {
		return nil, errors.New("caída")
	}
	return append([]client.Order(nil), a.orders...), nil
}

type fakeProcessor struct {
	mu                 sync.Mutex
	active, max, calls int
	delay              time.Duration
}

func (p *fakeProcessor) Process(ctx context.Context, o client.Order) error {
	p.mu.Lock()
	p.active++
	p.calls++
	if p.active > p.max {
		p.max = p.active
	}
	p.mu.Unlock()
	timer := time.NewTimer(p.delay)
	select {
	case <-ctx.Done():
		timer.Stop()
	case <-timer.C:
	}
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	return nil
}
func TestImmediatePeriodicConcurrencyRecovery(t *testing.T) {
	api := &fakeAPI{fail: 1, orders: []client.Order{{ID: 1, Status: "pending"}, {ID: 2, Status: "pending"}, {ID: 3, Status: "pending"}}}
	proc := &fakeProcessor{delay: 15 * time.Millisecond}
	state := NewState()
	w := New(api, proc, 10*time.Millisecond, 2, Backoff{Base: 5 * time.Millisecond, Max: 10 * time.Millisecond}, slog.New(slog.NewTextHandler(io.Discard, nil)), state)
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()
	w.Run(ctx)
	snap := w.Snapshot()
	if api.calls < 2 {
		t.Fatalf("calls=%d", api.calls)
	}
	if proc.max > 2 {
		t.Fatalf("max=%d", proc.max)
	}
	if snap.APIFailures < 1 || snap.Reconnections < 1 || snap.Status != StatusStopped {
		t.Fatalf("snapshot=%+v", snap)
	}
}
func TestBackoff(t *testing.T) {
	b := Backoff{Base: time.Second, Max: 5 * time.Second}
	if b.Duration(1) != time.Second || b.Duration(4) != 5*time.Second {
		t.Fatal("backoff incorrecto")
	}
}
