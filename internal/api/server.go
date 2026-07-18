package api

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/logging"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

type JobController interface {
	ListJobs() []supervisor.Snapshot
	Job(name string) (supervisor.Snapshot, error)
	StartJob(name string) error
	StopJob(name string) error
	RestartJob(name string) error
}

type LogReader interface {
	Read(job string, limit int) ([]logging.Entry, error)
}

type Server struct {
	http       *http.Server
	controller JobController
	logs       LogReader
	template   *template.Template
	logger     *log.Logger
}

func NewServer(address string, controller JobController, logs LogReader, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.Default()
	}
	tmpl, err := template.ParseGlob("web/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("cargar dashboard: %w", err)
	}
	s := &Server{controller: controller, logs: logs, template: tmpl, logger: logger}
	s.http = &http.Server{Addr: address, Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	return s, nil
}

func (s *Server) Start() error {
	s.logger.Printf("dashboard disponible en http://localhost%s", s.http.Addr)
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }
func (s *Server) Handler() http.Handler              { return s.http.Handler }
