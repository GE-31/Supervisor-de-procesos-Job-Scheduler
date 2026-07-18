package api

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/summary", s.handleSummary)
	mux.HandleFunc("/api/jobs", s.handleJobs)
	mux.HandleFunc("/api/jobs/", s.handleJob)
	mux.HandleFunc("/", s.handleDashboard)
	return s.recovery(s.accessLog(http.MaxBytesHandler(mux, 1<<20)))
}
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
func (s *Server) recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				s.logger.Printf("panic recuperado: %v", value)
				writeError(w, http.StatusInternalServerError, "internal_error", "Error interno del servidor")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
