package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// withRequestID asigna un identificador corto a cada solicitud, disponible
// para el resto de middleware y expuesto también al cliente en la cabecera
// X-Request-Id. Es opcional para el cliente pero facilita correlacionar logs.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(buf)
}

func requestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// statusRecorder captura el código de estado escrito por el handler para que
// el middleware de logging pueda registrarlo.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

// withLogging registra método, ruta, código de estado, duración y request ID
// de cada solicitud usando log/slog.
func withLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		logger.Info("solicitud HTTP",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration", time.Since(started),
			"request_id", requestIDFrom(r.Context()),
		)
	})
}

// withRecovery convierte cualquier panic del handler en un 500 controlado en
// lugar de tumbar el servidor completo.
func withRecovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				logger.Error("panic recuperado", "value", value, "path", r.URL.Path, "request_id", requestIDFrom(r.Context()))
				writeError(w, http.StatusInternalServerError, "internal_error", "Error interno del servidor")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// withSecurityHeaders añade cabeceras que endurecen al navegador contra MIME
// sniffing, framing y fugas de referrer, y una CSP que solo admite recursos
// del propio origen (nada de CDNs, coherente con el requisito sin Internet).
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
