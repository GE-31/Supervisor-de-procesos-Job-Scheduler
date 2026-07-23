package api

import (
	"bytes"
	"encoding/json"
	joblog "github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/logging"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeController struct {
	job    supervisor.Snapshot
	action string
}

func (f *fakeController) ListJobs() []supervisor.Snapshot { return []supervisor.Snapshot{f.job} }
func (f *fakeController) Job(name string) (supervisor.Snapshot, error) {
	if name != f.job.Name {
		return supervisor.Snapshot{}, supervisor.ErrNotFound
	}
	return f.job, nil
}
func (f *fakeController) StartJob(name string) error {
	if _, err := f.Job(name); err != nil {
		return err
	}
	f.action = "start"
	return nil
}
func (f *fakeController) StopJob(name string) error {
	if _, err := f.Job(name); err != nil {
		return err
	}
	f.action = "stop"
	return nil
}
func (f *fakeController) RestartJob(name string) error {
	if _, err := f.Job(name); err != nil {
		return err
	}
	f.action = "restart"
	return nil
}

type fakeLogs struct{}

func (fakeLogs) Read(string, int) ([]joblog.Entry, error) { return []joblog.Entry{}, nil }
func testHandler() (*fakeController, http.Handler) {
	c := &fakeController{job: supervisor.Snapshot{Name: "demo", Command: "echo", State: supervisor.StateRunning, PID: 42, RestartPolicy: "never", StartedAt: time.Now()}}
	s := &Server{controller: c, logs: fakeLogs{}, logger: log.New(io.Discard, "", 0)}
	return c, s.routes()
}

// repoRoot localiza la raíz del repositorio a partir de este archivo, para
// que template.ParseGlob("web/templates/...") resuelva igual que en
// producción, donde el binario se ejecuta desde la raíz.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no se pudo determinar la ruta del archivo de prueba")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// testHandlerWithPages construye un Server con las plantillas reales
// cargadas, para las pruebas que sí ejercitan las páginas HTML.
func testHandlerWithPages(t *testing.T) http.Handler {
	t.Helper()
	t.Chdir(repoRoot(t))
	tmpl, err := template.ParseGlob("web/templates/layouts/*.html")
	if err != nil {
		t.Fatalf("cargar layouts: %v", err)
	}
	tmpl, err = tmpl.ParseGlob("web/templates/pages/*.html")
	if err != nil {
		t.Fatalf("cargar pages: %v", err)
	}
	c := &fakeController{job: supervisor.Snapshot{Name: "demo", Command: "echo", State: supervisor.StateRunning, PID: 42, RestartPolicy: "never", StartedAt: time.Now()}}
	s := &Server{controller: c, logs: fakeLogs{}, template: tmpl, logger: log.New(io.Discard, "", 0)}
	return s.routes()
}
func request(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(nil))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestHealthAndJSON(t *testing.T) {
	_, h := testHandler()
	w := request(t, h, "GET", "/api/health")
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type=%s", got)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["status"] != "ok" {
		t.Fatalf("body=%s", w.Body.String())
	}
}
func TestSummaryAndJobs(t *testing.T) {
	_, h := testHandler()
	for _, path := range []string{"/api/summary", "/api/jobs", "/api/jobs/demo"} {
		w := request(t, h, "GET", path)
		if w.Code != 200 {
			t.Errorf("%s status=%d", path, w.Code)
		}
	}
}
func TestNotFoundAndMethod(t *testing.T) {
	_, h := testHandler()
	if w := request(t, h, "GET", "/api/jobs/missing"); w.Code != 404 {
		t.Fatalf("not found=%d", w.Code)
	}
	if w := request(t, h, "PUT", "/api/jobs"); w.Code != 405 {
		t.Fatalf("method=%d", w.Code)
	}
}
func TestActions(t *testing.T) {
	for _, action := range []string{"start", "stop", "restart"} {
		c, h := testHandler()
		w := request(t, h, "POST", "/api/jobs/demo/"+action)
		if w.Code != http.StatusAccepted {
			t.Errorf("%s status=%d", action, w.Code)
		}
		if c.action != action {
			t.Errorf("action=%q", c.action)
		}
	}
}
func TestLogsAndInvalidLimit(t *testing.T) {
	_, h := testHandler()
	if w := request(t, h, "GET", "/api/jobs/demo/logs?limit=50"); w.Code != 200 {
		t.Fatalf("logs=%d", w.Code)
	}
	if w := request(t, h, "GET", "/api/jobs/demo/logs?limit=5000"); w.Code != 400 {
		t.Fatalf("limit=%d", w.Code)
	}
}

// ---- páginas del dashboard ----
//
// El dashboard dejó de ser una sola página con secciones controladas por el
// hash de la URL: ahora son tres rutas HTTP reales, cada una con su propia
// plantilla. Estas pruebas comprueban que cada una renderiza su propio
// contenido, que no se mezclan entre sí, y que el enlace activo del sidebar
// cambia según la página.

func TestOverviewPageRenders(t *testing.T) {
	h := testHandlerWithPages(t)
	w := request(t, h, "GET", "/")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="stats"`) {
		t.Error("la página de resumen no contiene el panel de estadísticas")
	}
	if strings.Contains(body, `id="jobs-table"`) || strings.Contains(body, `id="log-console"`) {
		t.Error("la página de resumen no debería incluir la tabla de procesos ni la consola de logs")
	}
	if !strings.Contains(body, `href="/">Resumen`) {
		t.Error("el enlace de Resumen debería quedar marcado como activo")
	}
}

func TestProcessesPageRenders(t *testing.T) {
	h := testHandlerWithPages(t)
	w := request(t, h, "GET", "/processes")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="jobs-table"`) {
		t.Error("la página de procesos no contiene la tabla")
	}
	if strings.Contains(body, `id="stats"`) || strings.Contains(body, `id="log-console"`) {
		t.Error("la página de procesos no debería incluir el resumen ni la consola de logs")
	}
}

func TestLogsPageRenders(t *testing.T) {
	h := testHandlerWithPages(t)
	w := request(t, h, "GET", "/logs")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="log-console"`) || !strings.Contains(body, `id="log-job"`) {
		t.Error("la página de logs no contiene la consola ni el selector de proceso")
	}
	if strings.Contains(body, `id="stats"`) || strings.Contains(body, `id="jobs-table"`) {
		t.Error("la página de logs no debería incluir el resumen ni la tabla de procesos")
	}
}

func TestOverviewPageMethodNotAllowed(t *testing.T) {
	h := testHandlerWithPages(t)
	if w := request(t, h, "POST", "/"); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", w.Code)
	}
}
