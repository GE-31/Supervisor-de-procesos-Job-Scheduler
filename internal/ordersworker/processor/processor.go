package processor

import (
	"context"
	"fmt"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/client"
	"log/slog"
	"time"
)

type StatusClient interface {
	UpdateOrderStatus(context.Context, int64, string) error
}
type Processor struct {
	client   StatusClient
	duration time.Duration
	logger   *slog.Logger
}

func New(c StatusClient, duration time.Duration, logger *slog.Logger) *Processor {
	return &Processor{client: c, duration: duration, logger: logger}
}
func (p *Processor) Process(ctx context.Context, order client.Order) error {
	if order.Status != "pending" {
		return nil
	}
	started := time.Now()
	p.logger.Info("pedido detectado", "service", "orders-worker", "order_id", order.ID, "status", order.Status)
	if err := p.client.UpdateOrderStatus(ctx, order.ID, "processing"); err != nil {
		return fmt.Errorf("cambiar a processing: %w", err)
	}
	p.logger.Info("pedido en procesamiento", "service", "orders-worker", "order_id", order.ID)
	timer := time.NewTimer(p.duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		err = p.client.UpdateOrderStatus(ctx, order.ID, "completed")
		if err == nil {
			p.logger.Info("pedido completado", "service", "orders-worker", "order_id", order.ID, "customer", order.Customer, "product", order.Product, "duration", time.Since(started))
			return nil
		}
		timer.Reset(time.Duration(attempt) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("cambiar a completed después de 3 intentos: %w", err)
}
