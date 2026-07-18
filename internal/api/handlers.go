package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/api/dto"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not_found", "Recurso no encontrado")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	if err := s.template.ExecuteTemplate(w, "dashboard.html", nil); err != nil {
		s.logger.Printf("render dashboard: %v", err)
	}
}
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "job-scheduler"})
}
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	summary := dto.Summary{}
	for _, j := range s.controller.ListJobs() {
		summary.Total++
		summary.TotalRestarts += j.Retries
		switch j.State {
		case supervisor.StateRunning:
			summary.Running++
		case supervisor.StateBackoff:
			summary.Backoff++
		case supervisor.StateStopped:
			summary.Stopped++
		case supervisor.StateFailed:
			summary.Failed++
		}
	}
	writeJSON(w, http.StatusOK, summary)
}
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	jobs := s.controller.ListJobs()
	out := make([]dto.JobResponse, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, dto.FromSnapshot(j))
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Solicitud inválida")
		return
	}
	name := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		j, err := s.controller.Job(name)
		if handleControllerError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, dto.FromSnapshot(j))
		return
	}
	action := parts[1]
	if action == "logs" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		if _, err := s.controller.Job(name); handleControllerError(w, err) {
			return
		}
		limit := 200
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 1000 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "El límite debe estar entre 1 y 1000")
				return
			}
			limit = n
		}
		entries, err := s.logs.Read(name, limit)
		if err != nil {
			s.logger.Printf("leer logs de %s: %v", name, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "No se pudieron leer los logs")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job": name, "entries": entries})
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var err error
	switch action {
	case "start":
		err = s.controller.StartJob(name)
	case "stop":
		err = s.controller.StopJob(name)
	case "restart":
		err = s.controller.RestartJob(name)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Acción no encontrada")
		return
	}
	if handleControllerError(w, err) {
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "job": name, "action": action})
}
func handleControllerError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, supervisor.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job_not_found", "No existe el proceso solicitado")
		return true
	}
	if errors.Is(err, supervisor.ErrInvalidState) {
		writeError(w, http.StatusConflict, "invalid_state", "La acción no es válida para el estado actual")
		return true
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "Error interno del servidor")
	return true
}
