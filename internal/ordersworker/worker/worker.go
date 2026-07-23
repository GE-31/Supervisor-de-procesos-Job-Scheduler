package worker

import (
	"context"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/client"
	"log/slog"
	"sync"
	"time"
)

type API interface {
	Health(context.Context) error
	ListOrders(context.Context) ([]client.Order, error)
}
type OrderProcessor interface {
	Process(context.Context, client.Order) error
}
type Worker struct {
	api       API
	processor OrderProcessor
	interval  time.Duration
	backoff   Backoff
	logger    *slog.Logger
	state     *State
	sem       chan struct{}
	mu        sync.Mutex
	active    map[int64]struct{}
	wg        sync.WaitGroup
}

func New(api API, processor OrderProcessor, interval time.Duration, maxConcurrent int, backoff Backoff, logger *slog.Logger, state *State) *Worker {
	return &Worker{api: api, processor: processor, interval: interval, backoff: backoff, logger: logger, state: state, sem: make(chan struct{}, maxConcurrent), active: make(map[int64]struct{})}
}
func (w *Worker) Run(ctx context.Context) {
	defer func() { w.state.setStatus(StatusStopping); w.wg.Wait(); w.state.setStatus(StatusStopped) }()
	attempt := 0
	if err := w.api.Health(ctx); err != nil {
		w.state.apiFailure(err)
		w.logger.Warn("orders-api no disponible al iniciar", "service", "orders-worker", "error", err)
	} else {
		w.state.apiSuccess()
		w.logger.Info("conexión con orders-api establecida", "service", "orders-worker")
	}
	delay := time.Duration(0)
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		err := w.poll(ctx)
		if err != nil {
			attempt++
			delay = w.backoff.Duration(attempt)
			w.state.apiFailure(err)
			w.logger.Warn("consulta fallida; aplicando backoff", "service", "orders-worker", "state", StatusDegraded, "attempt", attempt, "duration", delay, "error", err)
		} else {
			if attempt > 0 {
				w.logger.Info("conexión con orders-api recuperada", "service", "orders-worker", "attempt", attempt)
			}
			attempt = 0
			delay = w.interval
			w.state.apiSuccess()
		}
	}
}
func (w *Worker) poll(ctx context.Context) error {
	orders, err := w.api.ListOrders(ctx)
	if err != nil {
		return err
	}
	for _, order := range orders {
		if order.Status != "pending" || !w.claim(order.ID) {
			continue
		}
		select {
		case w.sem <- struct{}{}:
			w.wg.Add(1)
			w.state.startedJob()
			go w.process(ctx, order)
		default:
			w.release(order.ID)
			return nil
		}
	}
	return nil
}
func (w *Worker) process(ctx context.Context, order client.Order) {
	defer w.wg.Done()
	defer func() { <-w.sem; w.release(order.ID) }()
	err := w.processor.Process(ctx, order)
	w.state.finishedJob(err)
	if err != nil {
		w.logger.Error("pedido fallido", "service", "orders-worker", "order_id", order.ID, "error", err)
	}
}
func (w *Worker) claim(id int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.active[id]; ok {
		return false
	}
	w.active[id] = struct{}{}
	return true
}
func (w *Worker) release(id int64)   { w.mu.Lock(); delete(w.active, id); w.mu.Unlock() }
func (w *Worker) Snapshot() Snapshot { return w.state.Snapshot() }
