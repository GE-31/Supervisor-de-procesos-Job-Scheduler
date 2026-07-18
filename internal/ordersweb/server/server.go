// Package server implementa el servidor HTTP de orders-web: sirve el
// dashboard, los archivos estáticos y un proxy explícito hacia orders-api.
// orders-web no almacena pedidos; toda la información viene del cliente HTTP
// hacia orders-api (internal/ordersweb/client).
package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/client"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/config"
)

const (
	templatesGlob = "web/orders/templates/*.html"
	staticDir     = "web/orders/static"
)

// CrashFunc termina el proceso con el código indicado. Es una función
// inyectable para que /demo/crash pueda probarse sin terminar el proceso de
// pruebas real (os.Exit por defecto fuera de tests).
type CrashFunc func(code int)

// Server agrupa el servidor HTTP de orders-web y sus dependencias: el
// cliente hacia orders-api, las plantillas ya cargadas y el logger.
type Server struct {
	http      *http.Server
	api       *client.Client
	apiURL    string
	templates *template.Template
	logger    *slog.Logger
	startedAt time.Time
	pid       int
	crash     CrashFunc
	refresh   time.Duration
}

// New construye el servidor: valida que las plantillas existan y compilen
// (evitando así iniciar con un dashboard roto) y arma su propio ServeMux con
// middleware. No usa http.DefaultServeMux ni http.DefaultClient en ningún punto.
func New(cfg config.Config, apiClient *client.Client, logger *slog.Logger, crash CrashFunc) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if crash == nil {
		crash = os.Exit
	}
	tmpl, err := template.ParseGlob(templatesGlob)
	if err != nil {
		return nil, fmt.Errorf("cargar plantillas de orders-web: %w", err)
	}
	s := &Server{
		api:       apiClient,
		apiURL:    cfg.OrdersAPIURL,
		templates: tmpl,
		logger:    logger,
		startedAt: time.Now(),
		pid:       os.Getpid(),
		crash:     crash,
		refresh:   cfg.DashboardRefresh,
	}
	s.http = &http.Server{
		Addr:              cfg.WebAddr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	return s, nil
}

// Start bloquea sirviendo HTTP hasta que el servidor se cierre. Un cierre
// ordenado vía Shutdown se reporta como nil, no como error.
func (s *Server) Start() error {
	s.logger.Info("orders-web escuchando", "address", s.http.Addr, "pid", s.pid, "orders_api_url", s.apiURL)
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown drena las solicitudes activas dentro del contexto dado y cierra
// el listener. Es la única vía de apagado normal; os.Exit queda reservado a
// /demo/crash.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
