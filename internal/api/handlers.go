package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/api/dto"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

// pageData es el valor pasado a cada una de las tres páginas del dashboard.
// Active identifica cuál queda resaltada en el sidebar ("overview",
// "processes" o "logs").
type pageData struct {
	Active   string
	Eyebrow  string
	Title    string
	Subtitle string
}

func (s *Server) renderPage(w http.ResponseWriter, tmplName string, data pageData) {
	if err := s.template.ExecuteTemplate(w, tmplName, data); err != nil {
		s.logger.Printf("render %s: %v", tmplName, err)
	}
}

// handleOverview sirve GET / : contadores agregados de todos los procesos.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not_found", "Recurso no encontrado")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	s.renderPage(w, "overview.html", pageData{
		Active:   "overview",
		Eyebrow:  "OPERACIONES / RESUMEN",
		Title:    "Estado general",
		Subtitle: "Contadores agregados de todos los procesos supervisados.",
	})
}

// handleProcesses sirve GET /processes : tabla de procesos supervisados.
func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	s.renderPage(w, "processes.html", pageData{
		Active:   "processes",
		Eyebrow:  "OPERACIONES / PROCESOS",
		Title:    "Procesos supervisados",
		Subtitle: "Solo se pueden controlar jobs definidos en la configuración.",
	})
}

// handleLogsPage sirve GET /logs : consola de logs por proceso.
func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	s.renderPage(w, "logs.html", pageData{
		Active:   "logs",
		Eyebrow:  "OPERACIONES / LOGS",
		Title:    "Consola de logs",
		Subtitle: "Últimas líneas de stdout y stderr por proceso.",
	})
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
	writeJSON(w, http.StatusOK, dto.SummaryFromSnapshots(s.controller.ListJobs()))
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
