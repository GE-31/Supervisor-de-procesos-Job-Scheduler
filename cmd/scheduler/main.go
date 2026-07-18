package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/api"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/config"
	joblog "github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/logging"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

func main() {
	configPath := flag.String("config", "configs/example.json", "ruta al archivo de configuración")
	flag.Parse()
	logger := log.New(os.Stdout, "scheduler ", log.LstdFlags|log.Lmicroseconds)
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatalf("cargar configuración: %v", err)
	}
	store, err := joblog.NewStore(cfg.LogDir)
	if err != nil {
		logger.Fatalf("inicializar logs: %v", err)
	}
	specs := make([]supervisor.JobSpec, 0, len(cfg.Processes))
	for _, p := range cfg.Processes {
		max := cfg.Backoff.MaxRetries
		if p.MaxRetries != nil {
			max = *p.MaxRetries
		}
		specs = append(specs, supervisor.JobSpec{Name: p.Name, Command: p.Command, Args: p.Args, Restart: p.Restart, WorkDir: p.WorkDir, MaxRetries: max})
	}
	sup, err := supervisor.New(specs, supervisor.Backoff{Base: cfg.Backoff.Base.Duration, Factor: cfg.Backoff.Factor, Max: cfg.Backoff.Max.Duration}, cfg.GracePeriod.Duration, store)
	if err != nil {
		logger.Fatalf("crear supervisor: %v", err)
	}
	server, err := api.NewServer(cfg.Address, sup, store, logger)
	if err != nil {
		logger.Fatalf("crear servidor: %v", err)
	}
	sup.StartAll()
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Start() }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		logger.Printf("señal %s recibida; iniciando apagado", sig)
	case err := <-serverErr:
		if err != nil {
			logger.Printf("servidor HTTP: %v", err)
		}
	}
	signal.Stop(signals)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.GracePeriod.Duration+5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("apagar servidor HTTP: %v", err)
	}
	if err := sup.Shutdown(ctx); err != nil {
		logger.Printf("apagar supervisor: %v", err)
	}
	logger.Print("apagado completo")
}
