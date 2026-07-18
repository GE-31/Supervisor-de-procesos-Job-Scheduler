package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	for _, name := range []string{"ORDERS_API_URL", "ORDERS_WORKER_INTERVAL", "ORDERS_WORKER_PROCESSING_TIME", "ORDERS_WORKER_MAX_CONCURRENT", "ORDERS_WORKER_REQUEST_TIMEOUT", "ORDERS_WORKER_BACKOFF_BASE", "ORDERS_WORKER_BACKOFF_MAX", "ORDERS_WORKER_HEALTH_ADDR", "ORDERS_WORKER_SHUTDOWN_TIMEOUT"} {
		t.Setenv(name, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIURL != "http://localhost:9091" || c.Interval != 5*time.Second || c.MaxConcurrent != 2 || c.HealthAddr != "127.0.0.1:9093" {
		t.Fatalf("defaults=%+v", c)
	}
}
func TestCustomAndInvalid(t *testing.T) {
	t.Setenv("ORDERS_API_URL", "https://example.test")
	t.Setenv("ORDERS_WORKER_INTERVAL", "20ms")
	t.Setenv("ORDERS_WORKER_MAX_CONCURRENT", "4")
	c, err := Load()
	if err != nil || c.Interval != 20*time.Millisecond || c.MaxConcurrent != 4 {
		t.Fatalf("config=%+v err=%v", c, err)
	}
	c.APIURL = ":bad"
	if Validate(c) == nil {
		t.Fatal("URL inválida aceptada")
	}
	c.APIURL = "http://ok.test"
	c.MaxConcurrent = 0
	if Validate(c) == nil {
		t.Fatal("concurrencia inválida aceptada")
	}
	c.MaxConcurrent = 1
	c.BackoffBase = time.Second
	c.BackoffMax = time.Millisecond
	if Validate(c) == nil {
		t.Fatal("backoff inválido aceptado")
	}
}
