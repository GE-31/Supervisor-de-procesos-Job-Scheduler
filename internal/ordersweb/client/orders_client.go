package client

import (
	"context"
	"fmt"
	"net/http"
)

// Health consulta GET /health en orders-api.
func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var out HealthResponse
	if err := c.do(ctx, http.MethodGet, "/health", nil, &out); err != nil {
		return HealthResponse{}, err
	}
	return out, nil
}

// ListOrders consulta GET /api/orders.
func (c *Client) ListOrders(ctx context.Context) (OrdersList, error) {
	var out OrdersList
	if err := c.do(ctx, http.MethodGet, "/api/orders", nil, &out); err != nil {
		return OrdersList{}, err
	}
	return out, nil
}

// CreateOrder envía POST /api/orders. La validación de negocio (campos
// obligatorios, cantidad y precio positivos, cálculo del total) la realiza
// exclusivamente orders-api; aquí solo se reenvía la solicitud.
func (c *Client) CreateOrder(ctx context.Context, input CreateOrderRequest) (Order, error) {
	var out Order
	if err := c.do(ctx, http.MethodPost, "/api/orders", input, &out); err != nil {
		return Order{}, err
	}
	return out, nil
}

// GetOrder consulta GET /api/orders/{id}.
func (c *Client) GetOrder(ctx context.Context, id int64) (Order, error) {
	var out Order
	path := fmt.Sprintf("/api/orders/%d", id)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return Order{}, err
	}
	return out, nil
}

// UpdateOrderStatus envía PATCH /api/orders/{id}/status.
func (c *Client) UpdateOrderStatus(ctx context.Context, id int64, status string) (Order, error) {
	var out Order
	path := fmt.Sprintf("/api/orders/%d/status", id)
	if err := c.do(ctx, http.MethodPatch, path, UpdateStatusRequest{Status: status}, &out); err != nil {
		return Order{}, err
	}
	return out, nil
}

// DeleteOrder envía DELETE /api/orders/{id}. orders-api responde 204 sin
// cuerpo, por lo que no se decodifica nada.
func (c *Client) DeleteOrder(ctx context.Context, id int64) error {
	path := fmt.Sprintf("/api/orders/%d", id)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// GetStats consulta GET /api/orders/stats.
func (c *Client) GetStats(ctx context.Context) (Stats, error) {
	var out Stats
	if err := c.do(ctx, http.MethodGet, "/api/orders/stats", nil, &out); err != nil {
		return Stats{}, err
	}
	return out, nil
}
