package main

import (
	"context"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/client"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/config"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/health"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/processor"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersworker/worker"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuración inválida", "service", "orders-worker", "error", err)
		os.Exit(1)
	}
	api, err := client.New(cfg.APIURL, cfg.RequestTimeout)
	if err != nil {
		logger.Error("crear cliente", "error", err)
		os.Exit(1)
	}
	state := worker.NewState()
	proc := processor.New(api, cfg.ProcessingTime, logger)
	work := worker.New(api, proc, cfg.Interval, cfg.MaxConcurrent, worker.Backoff{Base: cfg.BackoffBase, Max: cfg.BackoffMax}, logger, state)
	admin := health.New(cfg.HealthAddr, work, logger, os.Exit)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	workerDone := make(chan struct{})
	go func() { work.Run(ctx); close(workerDone) }()
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("orders-worker iniciado", "service", "orders-worker", "pid", os.Getpid(), "api_url", cfg.APIURL, "health_addr", cfg.HealthAddr, "max_concurrent", cfg.MaxConcurrent)
		serverErr <- admin.Start()
	}()
	select {
	case <-ctx.Done():
		logger.Info("señal recibida; iniciando apagado", "service", "orders-worker")
	case err := <-serverErr:
		if err != nil {
			logger.Error("health server falló", "error", err)
		}
		stop()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	_ = admin.Shutdown(shutdownCtx)
	select {
	case <-workerDone:
		logger.Info("apagado ordenado finalizado", "service", "orders-worker")
	case <-shutdownCtx.Done():
		logger.Error("timeout durante apagado", "service", "orders-worker")
	}
}
