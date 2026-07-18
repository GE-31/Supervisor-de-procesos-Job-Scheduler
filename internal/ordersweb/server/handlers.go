package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/client"
)

// maxRequestBody limita el cuerpo de las solicitudes que orders-web acepta
// desde el navegador (POST/PATCH del proxy).
const maxRequestBody = 1 << 20 // 1 MiB

// dashboardData es el único valor pasado a la plantilla del dashboard.
type dashboardData struct {
	OrdersAPIURL   string
	RefreshSeconds int
	StatusOptions  []string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := dashboardData{
		OrdersAPIURL:   s.apiURL,
		RefreshSeconds: int(s.refresh / time.Second),
		StatusOptions:  []string{client.StatusPending, client.StatusProcessing, client.StatusCompleted, client.StatusCancelled},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		s.logger.Error("renderizar dashboard", "error", err)
	}
}

// handleHealth es el /health propio de orders-web (no confundir con
// /proxy/health, que refleja el estado de orders-api).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"service":        "orders-web",
		"pid":            s.pid,
		"time":           time.Now(),
		"uptime_seconds": int64(time.Since(s.startedAt).Seconds()),
		"orders_api_url": s.apiURL,
	})
}

func (s *Server) proxyHealth(w http.ResponseWriter, r *http.Request) {
	health, err := s.api.Health(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, health)
}

func (s *Server) proxyOrdersList(w http.ResponseWriter, r *http.Request) {
	list, err := s.api.ListOrders(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) proxyOrdersCreate(w http.ResponseWriter, r *http.Request) {
	if !requireJSONContentType(w, r) {
		return
	}
	var input client.CreateOrderRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	order, err := s.api.CreateOrder(r.Context(), input)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (s *Server) proxyOrderGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "El identificador del pedido debe ser numérico y positivo")
		return
	}
	order, err := s.api.GetOrder(r.Context(), id)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) proxyOrderDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "El identificador del pedido debe ser numérico y positivo")
		return
	}
	if err := s.api.DeleteOrder(r.Context(), id); err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) proxyOrderStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "El identificador del pedido debe ser numérico y positivo")
		return
	}
	if !requireJSONContentType(w, r) {
		return
	}
	var input client.UpdateStatusRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	order, err := s.api.UpdateOrderStatus(r.Context(), id, input.Status)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) proxyStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.api.GetStats(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// demoCrash existe únicamente para demostrar la supervisión: nunca se llama
// a sí misma, requiere una solicitud POST explícita del operador.
func (s *Server) demoCrash(w http.ResponseWriter, r *http.Request) {
	s.logger.Error("caída de demostración solicitada", "pid", s.pid)
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"message": "Caída de demostración iniciada; orders-web va a terminar",
		"pid":     s.pid,
	})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.logger.Error("terminando orders-web por caída de demostración", "pid", s.pid)
		s.crash(1)
	}()
}

// writeUpstreamError traduce los errores del cliente hacia orders-api en la
// respuesta HTTP que ve el navegador, sin filtrar detalles internos.
func (s *Server) writeUpstreamError(w http.ResponseWriter, err error) {
	var apiErr *client.APIError
	switch {
	case errors.As(err, &apiErr):
		writeError(w, apiErr.StatusCode, apiErr.Code, apiErr.Message)
	case errors.Is(err, client.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "orders_api_unavailable", "El servidor de pedidos no está disponible")
	case errors.Is(err, client.ErrTimeout):
		writeError(w, http.StatusGatewayTimeout, "orders_api_timeout", "El servidor de pedidos no respondió a tiempo")
	case errors.Is(err, client.ErrBadGateway):
		writeError(w, http.StatusBadGateway, "orders_api_bad_gateway", "El servidor de pedidos respondió de forma inesperada")
	default:
		s.logger.Error("error inesperado al comunicarse con orders-api", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Error interno del servidor")
	}
}

func requireJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		writeError(w, http.StatusBadRequest, "invalid_content_type", "El cuerpo debe enviarse como application/json")
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "El cuerpo JSON no es válido")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "Solo se permite un objeto JSON")
		return false
	}
	return true
}

func parsePositiveID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}
