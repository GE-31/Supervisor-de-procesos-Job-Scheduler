// orders-web es la interfaz web del sistema de pedidos. Es un programa
// independiente de orders-api: consulta su información exclusivamente a
// través de HTTP y no almacena pedidos propios. Este archivo es el
// composition root: solo conecta configuración, cliente, servidor y señales.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/client"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/config"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("cargar configuración", "error", err)
		os.Exit(1)
	}

	apiClient, err := client.New(cfg.OrdersAPIURL, cfg.ClientTimeout)
	if err != nil {
		logger.Error("crear cliente de orders-api", "error", err)
		os.Exit(1)
	}

	srv, err := server.New(cfg, apiClient, logger, os.Exit)
	if err != nil {
		logger.Error("crear servidor de orders-web", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Start() }()

	select {
	case <-ctx.Done():
		logger.Info("señal recibida; iniciando apagado ordenado")
	case err := <-serverErr:
		if err != nil {
			logger.Error("servidor HTTP detenido con error", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("error durante apagado", "error", err)
		os.Exit(1)
	}
	logger.Info("apagado ordenado finalizado")
}
