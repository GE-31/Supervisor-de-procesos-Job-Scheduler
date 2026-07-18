package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/client"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/ordersweb/config"
)

// repoRoot localiza la raíz del repositorio a partir de este archivo, para
// que las plantillas y los estáticos (rutas relativas) se resuelvan igual
// que en producción, donde el binario se ejecuta desde la raíz.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no se pudo determinar la ruta del archivo de prueba")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type crashRecorder struct {
	mu    sync.Mutex
	code  int
	calls int
}

func (c *crashRecorder) crash(code int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.code = code
	c.calls++
}

func (c *crashRecorder) waitForCall(t *testing.T) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		calls := c.calls
		code := c.code
		c.mu.Unlock()
		if calls > 0 {
			return code
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("se esperaba que crash() fuera invocado")
	return 0
}

func newTestServer(t *testing.T, apiURL string, crash CrashFunc) *Server {
	t.Helper()
	return newTestServerWithLogger(t, apiURL, crash, silentLogger())
}

func newTestServerWithLogger(t *testing.T, apiURL string, crash CrashFunc, logger *slog.Logger) *Server {
	t.Helper()
	t.Chdir(repoRoot(t))
	apiClient, err := client.New(apiURL, time.Second)
	if err != nil {
		t.Fatalf("client.New() error: %v", err)
	}
	cfg := config.Config{WebAddr: ":0", OrdersAPIURL: apiURL, ClientTimeout: time.Second, ShutdownGrace: time.Second, DashboardRefresh: 5 * time.Second}
	srv, err := New(cfg, apiClient, logger, crash)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return srv
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("decodificar cuerpo %q: %v", w.Body.String(), err)
	}
}

// ---- plantillas y estáticos ----

func TestHandleIndexRendersDashboard(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	w := doRequest(t, srv.routes(), http.MethodGet, "/", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), "Orders Control") {
		t.Error("el dashboard no contiene el nombre visible esperado")
	}
	if !strings.Contains(w.Body.String(), "Registrar pedido") {
		t.Error("el dashboard no contiene el formulario de registro")
	}
}

func TestStaticAssetsServed(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	w := doRequest(t, srv.routes(), http.MethodGet, "/assets/css/variables.css", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "--color-primary") {
		t.Error("el CSS servido no contiene las variables esperadas")
	}
}

// ---- /health propio ----

func TestOwnHealth(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	w := doRequest(t, srv.routes(), http.MethodGet, "/health", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var out struct {
		Status  string `json:"status"`
		Service string `json:"service"`
		PID     int    `json:"pid"`
	}
	decodeBody(t, w, &out)
	if out.Status != "ok" || out.Service != "orders-web" || out.PID <= 0 {
		t.Errorf("/health = %+v", out)
	}
}

// ---- proxy: orders-api disponible ----

func fakeOrdersAPI(t *testing.T, mux func(mux *http.ServeMux)) *httptest.Server {
	t.Helper()
	m := http.NewServeMux()
	mux(m)
	server := httptest.NewServer(m)
	t.Cleanup(server.Close)
	return server
}

func TestProxyHealthSuccess(t *testing.T) {
	api := fakeOrdersAPI(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"status": "ok", "service": "orders-api", "pid": 1})
		})
	})
	srv := newTestServer(t, api.URL, nil)
	w := doRequest(t, srv.routes(), http.MethodGet, "/proxy/health", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestProxyOrdersListAndStats(t *testing.T) {
	api := fakeOrdersAPI(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /api/orders", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "count": 0})
		})
		m.HandleFunc("GET /api/orders/stats", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"total_orders": 0})
		})
	})
	srv := newTestServer(t, api.URL, nil)
	if w := doRequest(t, srv.routes(), http.MethodGet, "/proxy/orders", nil, ""); w.Code != http.StatusOK {
		t.Fatalf("/proxy/orders status = %d", w.Code)
	}
	if w := doRequest(t, srv.routes(), http.MethodGet, "/proxy/orders/stats", nil, ""); w.Code != http.StatusOK {
		t.Fatalf("/proxy/orders/stats status = %d", w.Code)
	}
}

func TestProxyOrdersCreateSuccess(t *testing.T) {
	api := fakeOrdersAPI(t, func(m *http.ServeMux) {
		m.HandleFunc("POST /api/orders", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(client.Order{ID: 1, Customer: "Ana", Total: 20})
		})
	})
	srv := newTestServer(t, api.URL, nil)
	body, _ := json.Marshal(client.CreateOrderRequest{Customer: "Ana", Product: "Mouse", Quantity: 2, UnitPrice: 10})
	w := doRequest(t, srv.routes(), http.MethodPost, "/proxy/orders", body, "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestProxyOrdersCreateValidationPassthrough(t *testing.T) {
	api := fakeOrdersAPI(t, func(m *http.ServeMux) {
		m.HandleFunc("POST /api/orders", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "validation_error", "message": "La cantidad debe ser mayor que cero"})
		})
	})
	srv := newTestServer(t, api.URL, nil)
	body, _ := json.Marshal(client.CreateOrderRequest{Customer: "Ana", Product: "Mouse", Quantity: 0, UnitPrice: 10})
	w := doRequest(t, srv.routes(), http.MethodPost, "/proxy/orders", body, "application/json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
	var out struct{ Error string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Error != "validation_error" {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestProxyOrdersCreateInvalidContentType(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	w := doRequest(t, srv.routes(), http.MethodPost, "/proxy/orders", []byte("{}"), "text/plain")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestProxyOrdersCreateInvalidJSON(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	w := doRequest(t, srv.routes(), http.MethodPost, "/proxy/orders", []byte("no es json"), "application/json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestProxyOrderGetInvalidID(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	for _, id := range []string{"abc", "-1", "0"} {
		w := doRequest(t, srv.routes(), http.MethodGet, "/proxy/orders/"+id, nil, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("id=%q status = %d", id, w.Code)
		}
	}
}

func TestProxyOrderDeleteSuccess(t *testing.T) {
	api := fakeOrdersAPI(t, func(m *http.ServeMux) {
		m.HandleFunc("DELETE /api/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})
	srv := newTestServer(t, api.URL, nil)
	w := doRequest(t, srv.routes(), http.MethodDelete, "/proxy/orders/9", nil, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestProxyOrderStatusUpdate(t *testing.T) {
	api := fakeOrdersAPI(t, func(m *http.ServeMux) {
		m.HandleFunc("PATCH /api/orders/{id}/status", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(client.Order{ID: 7, Status: "processing"})
		})
	})
	srv := newTestServer(t, api.URL, nil)
	body, _ := json.Marshal(client.UpdateStatusRequest{Status: "processing"})
	w := doRequest(t, srv.routes(), http.MethodPatch, "/proxy/orders/7/status", body, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// ---- orders-api desconectada ----

func TestProxyWhenOrdersAPIUnavailable(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	for _, path := range []string{"/proxy/health", "/proxy/orders", "/proxy/orders/stats"} {
		w := doRequest(t, srv.routes(), http.MethodGet, path, nil, "")
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, body = %s", path, w.Code, w.Body.String())
		}
		var out struct{ Error string }
		json.Unmarshal(w.Body.Bytes(), &out)
		if out.Error != "orders_api_unavailable" {
			t.Errorf("%s error = %q", path, out.Error)
		}
	}
	// El dashboard debe seguir cargando aunque orders-api esté caída.
	if w := doRequest(t, srv.routes(), http.MethodGet, "/", nil, ""); w.Code != http.StatusOK {
		t.Fatalf("GET / con orders-api caída, status = %d", w.Code)
	}
}

// ---- método no permitido, content-type, seguridad ----

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	w := doRequest(t, srv.routes(), http.MethodDelete, "/proxy/orders", nil, "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", w.Code)
	}
}

// TestRequestIDPropagatesToLog cubre una regresión real: si withRequestID
// se coloca por fuera de withLogging en la cadena de middleware, el logger
// lee el contexto de la solicitud original (sin ID) y siempre registra
// request_id="". El ID sí debe llegar tanto a la cabecera de respuesta como
// a la línea de log.
func TestRequestIDPropagatesToLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	srv := newTestServerWithLogger(t, "http://127.0.0.1:1", nil, logger)

	w := doRequest(t, srv.routes(), http.MethodGet, "/health", nil, "")

	headerID := w.Header().Get("X-Request-Id")
	if headerID == "" {
		t.Fatal("X-Request-Id no se estableció en la respuesta")
	}
	if !strings.Contains(buf.String(), "request_id="+headerID) {
		t.Errorf("el log no contiene request_id=%s: %s", headerID, buf.String())
	}
	if strings.Contains(buf.String(), `request_id=""`) {
		t.Errorf("el log registró un request_id vacío: %s", buf.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", nil)
	w := doRequest(t, srv.routes(), http.MethodGet, "/health", nil, "")
	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, expected := range headers {
		if got := w.Header().Get(header); got != expected {
			t.Errorf("%s = %q, se esperaba %q", header, got, expected)
		}
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
}

// ---- recuperación de panic ----

func TestRecoveryMiddleware(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	handler := withRecovery(silentLogger(), panicking)
	w := doRequest(t, handler, http.MethodGet, "/", nil, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "internal_error") {
		t.Errorf("body = %s", w.Body.String())
	}
}

// ---- /demo/crash ----

func TestDemoCrashRespondsAndCrashes(t *testing.T) {
	recorder := &crashRecorder{}
	srv := newTestServer(t, "http://127.0.0.1:1", recorder.crash)
	w := doRequest(t, srv.routes(), http.MethodPost, "/demo/crash", nil, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	var out struct {
		PID int `json:"pid"`
	}
	decodeBody(t, w, &out)
	if out.PID <= 0 {
		t.Errorf("pid = %d", out.PID)
	}
	if code := recorder.waitForCall(t); code != 1 {
		t.Errorf("crash() invocado con código %d, se esperaba 1", code)
	}
}

func TestDemoCrashRejectsGet(t *testing.T) {
	srv := newTestServer(t, "http://127.0.0.1:1", func(int) {})
	w := doRequest(t, srv.routes(), http.MethodGet, "/demo/crash", nil, "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", w.Code)
	}
}

// ---- concurrencia ----

func TestConcurrentProxyRequests(t *testing.T) {
	api := fakeOrdersAPI(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /api/orders", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "count": 0})
		})
	})
	srv := newTestServer(t, api.URL, nil)
	handler := srv.routes()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := doRequest(t, handler, http.MethodGet, "/proxy/orders", nil, "")
			if w.Code != http.StatusOK {
				t.Errorf("status = %d", w.Code)
			}
		}()
	}
	wg.Wait()
}
