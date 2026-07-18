package client

import (
	"errors"
	"fmt"
	"time"
)

// Estados válidos de un pedido, replicados desde orders-api para que el
// frontend y los handlers de orders-web puedan validar sin otra dependencia.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
)

// Order es el DTO de un pedido tal como lo entrega orders-api.
type Order struct {
	ID        int64     `json:"id"`
	Customer  string    `json:"customer"`
	Product   string    `json:"product"`
	Quantity  int       `json:"quantity"`
	UnitPrice float64   `json:"unit_price"`
	Total     float64   `json:"total"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Stats resume el estado agregado de los pedidos.
type Stats struct {
	TotalOrders int     `json:"total_orders"`
	Pending     int     `json:"pending"`
	Processing  int     `json:"processing"`
	Completed   int     `json:"completed"`
	Cancelled   int     `json:"cancelled"`
	TotalUnits  int     `json:"total_units"`
	TotalAmount float64 `json:"total_amount"`
}

// OrdersList es la envoltura que devuelve GET /api/orders.
type OrdersList struct {
	Data  []Order `json:"data"`
	Count int     `json:"count"`
}

// CreateOrderRequest es el cuerpo esperado por POST /api/orders.
type CreateOrderRequest struct {
	Customer  string  `json:"customer"`
	Product   string  `json:"product"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

// UpdateStatusRequest es el cuerpo esperado por PATCH /api/orders/{id}/status.
type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// HealthResponse refleja GET /health de orders-api.
type HealthResponse struct {
	Status        string    `json:"status"`
	Service       string    `json:"service"`
	PID           int       `json:"pid"`
	Time          time.Time `json:"time"`
	UptimeSeconds int64     `json:"uptime_seconds"`
}

// APIError representa una respuesta de error que orders-api sí llegó a
// producir (incluye código HTTP y el cuerpo {error, message}). Los handlers
// de orders-web la reenvían tal cual, porque ya tiene el formato acordado.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("orders-api respondió %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// Errores de conectividad: ocurren cuando la respuesta de orders-api no pudo
// obtenerse o interpretarse en absoluto, a diferencia de APIError.
var (
	// ErrUnavailable indica que no fue posible conectar con orders-api.
	ErrUnavailable = errors.New("orders-api no está disponible")
	// ErrTimeout indica que la solicitud excedió el tiempo de espera configurado.
	ErrTimeout = errors.New("tiempo de espera agotado al conectar con orders-api")
	// ErrBadGateway indica que orders-api respondió de forma inesperada
	// (cuerpo no JSON, tamaño excesivo o formato desconocido).
	ErrBadGateway = errors.New("orders-api respondió de forma inesperada")
)
