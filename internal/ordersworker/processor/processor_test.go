package processor

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

type updater struct {
	mu       sync.Mutex
	statuses []string
	err      error
}

func (u *updater) UpdateOrderStatus(ctx context.Context, id int64, status string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.statuses = append(u.statuses, status)
	return u.err
}
func TestProcess(t *testing.T) {
	u := &updater{}
	p := New(u, 5*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := p.Process(context.Background(), client.Order{ID: 1, Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if len(u.statuses) != 2 || u.statuses[0] != "processing" || u.statuses[1] != "completed" {
		t.Fatalf("statuses=%v", u.statuses)
	}
	before := len(u.statuses)
	_ = p.Process(context.Background(), client.Order{ID: 2, Status: "completed"})
	if len(u.statuses) != before {
		t.Fatal("procesó pedido no pending")
	}
}
func TestProcessErrorAndCancel(t *testing.T) {
	u := &updater{err: errors.New("fallo")}
	p := New(u, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := p.Process(context.Background(), client.Order{ID: 1, Status: "pending"}); err == nil {
		t.Fatal("se esperaba error")
	}
	u.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Process(ctx, client.Order{ID: 2, Status: "pending"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
