package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	if _, err := New("://no-valida", time.Second); err == nil {
		t.Fatal("se esperaba error para baseURL malformada")
	}
	if _, err := New("ftp://localhost:9091", time.Second); err == nil {
		t.Fatal("se esperaba error para esquema no http/https")
	}
	if _, err := New("http://", time.Second); err == nil {
		t.Fatal("se esperaba error para host vacío")
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := New(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("New() error inesperado: %v", err)
	}
	return c
}

func TestHealth(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" || r.Method != http.MethodGet {
			t.Errorf("solicitud inesperada: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok", Service: "orders-api", PID: 123, UptimeSeconds: 10})
	})
	got, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if got.Status != "ok" || got.PID != 123 {
		t.Errorf("Health() = %+v", got)
	}
}

func TestListOrders(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(OrdersList{Data: []Order{{ID: 1, Customer: "Ana"}}, Count: 1})
	})
	got, err := c.ListOrders(context.Background())
	if err != nil {
		t.Fatalf("ListOrders() error: %v", err)
	}
	if got.Count != 1 || len(got.Data) != 1 || got.Data[0].Customer != "Ana" {
		t.Errorf("ListOrders() = %+v", got)
	}
}

func TestCreateOrder(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/orders" {
			t.Errorf("solicitud inesperada: %s %s", r.Method, r.URL.Path)
		}
		var input CreateOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decodificar cuerpo: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Order{ID: 5, Customer: input.Customer, Quantity: input.Quantity, UnitPrice: input.UnitPrice, Total: float64(input.Quantity) * input.UnitPrice})
	})
	got, err := c.CreateOrder(context.Background(), CreateOrderRequest{Customer: "Luis", Product: "Teclado", Quantity: 2, UnitPrice: 10})
	if err != nil {
		t.Fatalf("CreateOrder() error: %v", err)
	}
	if got.ID != 5 || got.Total != 20 {
		t.Errorf("CreateOrder() = %+v", got)
	}
}

func TestGetOrder(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orders/42" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Order{ID: 42})
	})
	got, err := c.GetOrder(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetOrder() error: %v", err)
	}
	if got.ID != 42 {
		t.Errorf("GetOrder() = %+v", got)
	}
}

func TestUpdateOrderStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/orders/7/status" {
			t.Errorf("solicitud inesperada: %s %s", r.Method, r.URL.Path)
		}
		var input UpdateStatusRequest
		json.NewDecoder(r.Body).Decode(&input)
		json.NewEncoder(w).Encode(Order{ID: 7, Status: input.Status})
	})
	got, err := c.UpdateOrderStatus(context.Background(), 7, StatusProcessing)
	if err != nil {
		t.Fatalf("UpdateOrderStatus() error: %v", err)
	}
	if got.Status != StatusProcessing {
		t.Errorf("UpdateOrderStatus() = %+v", got)
	}
}

func TestDeleteOrder(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/orders/9" {
			t.Errorf("solicitud inesperada: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.DeleteOrder(context.Background(), 9); err != nil {
		t.Fatalf("DeleteOrder() error: %v", err)
	}
}

func TestGetStats(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Stats{TotalOrders: 3, Pending: 1})
	})
	got, err := c.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats() error: %v", err)
	}
	if got.TotalOrders != 3 {
		t.Errorf("GetStats() = %+v", got)
	}
}

func TestAPIErrorPassthrough(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "order_not_found", "message": "No existe el pedido solicitado"})
	})
	_, err := c.GetOrder(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("se esperaba *APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusNotFound || apiErr.Code != "order_not_found" {
		t.Errorf("APIError = %+v", apiErr)
	}
}

func TestInvalidJSONResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("no es json"))
	})
	_, err := c.ListOrders(context.Background())
	if !errors.Is(err, ErrBadGateway) {
		t.Fatalf("se esperaba ErrBadGateway, got %v", err)
	}
}

func TestUnreachableServer(t *testing.T) {
	c, err := New("http://127.0.0.1:1", time.Second)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	_, err = c.Health(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("se esperaba ErrUnavailable, got %v", err)
	}
}

func TestTimeout(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
	})
	c.httpClient.Timeout = 10 * time.Millisecond
	_, err := c.Health(context.Background())
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("se esperaba ErrTimeout, got %v", err)
	}
}

func TestResponseTooLarge(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("a", maxResponseBody+1)))
	})
	_, err := c.ListOrders(context.Background())
	if !errors.Is(err, ErrBadGateway) {
		t.Fatalf("se esperaba ErrBadGateway por tamaño excesivo, got %v", err)
	}
}

func TestErrorWithoutRecognizableBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>error</html>"))
	})
	_, err := c.ListOrders(context.Background())
	if !errors.Is(err, ErrBadGateway) {
		t.Fatalf("se esperaba ErrBadGateway, got %v", err)
	}
}
