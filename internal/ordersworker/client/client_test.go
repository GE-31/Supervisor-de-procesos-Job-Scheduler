package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serverClient(t *testing.T, handler http.HandlerFunc, timeout time.Duration) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(server.URL, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func TestHealthListAndUpdate(t *testing.T) {
	calls := 0
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			w.Write([]byte(`{"status":"ok"}`))
		case "/api/orders":
			w.Write([]byte(`{"data":[{"id":1,"status":"pending"}],"count":1}`))
		case "/api/orders/1/status":
			if r.Method != http.MethodPatch {
				t.Errorf("method=%s", r.Method)
			}
			w.Write([]byte(`{"id":1}`))
		}
	}, time.Second)
	if err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	orders, err := c.ListOrders(context.Background())
	if err != nil || len(orders) != 1 {
		t.Fatalf("orders=%v err=%v", orders, err)
	}
	if err := c.UpdateOrderStatus(context.Background(), 1, "completed"); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestClientErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{{"json", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{")) }, nil}, {"http", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }, ErrTemporary}, {"large", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", maxResponseSize+1))) }, ErrResponseTooLarge}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := serverClient(t, tc.handler, time.Second)
			_, err := c.ListOrders(context.Background())
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error=%v", err)
			}
			if tc.want == nil && err == nil {
				t.Fatal("se esperaba error")
			}
		})
	}
}
func TestTimeoutAndCancellation(t *testing.T) {
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(100 * time.Millisecond) }, 10*time.Millisecond)
	if err := c.Health(context.Background()); !errors.Is(err, ErrTimeout) {
		t.Fatalf("error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Health(ctx); !errors.Is(err, ErrTimeout) {
		t.Fatalf("cancel error=%v", err)
	}
}
func TestUnavailable(t *testing.T) {
	c, err := New("http://127.0.0.1:1", 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error=%v", err)
	}
}
