package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	APIURL                                                                             string
	Interval, ProcessingTime, RequestTimeout, BackoffBase, BackoffMax, ShutdownTimeout time.Duration
	MaxConcurrent                                                                      int
	HealthAddr                                                                         string
}

func Load() (Config, error) {
	c := Config{APIURL: get("ORDERS_API_URL", "http://localhost:9091"), HealthAddr: get("ORDERS_WORKER_HEALTH_ADDR", "127.0.0.1:9093")}
	var err error
	if c.Interval, err = duration("ORDERS_WORKER_INTERVAL", 5*time.Second); err != nil {
		return c, err
	}
	if c.ProcessingTime, err = duration("ORDERS_WORKER_PROCESSING_TIME", 3*time.Second); err != nil {
		return c, err
	}
	if c.RequestTimeout, err = duration("ORDERS_WORKER_REQUEST_TIMEOUT", 5*time.Second); err != nil {
		return c, err
	}
	if c.BackoffBase, err = duration("ORDERS_WORKER_BACKOFF_BASE", time.Second); err != nil {
		return c, err
	}
	if c.BackoffMax, err = duration("ORDERS_WORKER_BACKOFF_MAX", 30*time.Second); err != nil {
		return c, err
	}
	if c.ShutdownTimeout, err = duration("ORDERS_WORKER_SHUTDOWN_TIMEOUT", 5*time.Second); err != nil {
		return c, err
	}
	raw := get("ORDERS_WORKER_MAX_CONCURRENT", "2")
	c.MaxConcurrent, err = strconv.Atoi(raw)
	if err != nil {
		return c, fmt.Errorf("ORDERS_WORKER_MAX_CONCURRENT inválido: %w", err)
	}
	return c, Validate(c)
}
func Validate(c Config) error {
	u, err := url.ParseRequestURI(c.APIURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("ORDERS_API_URL debe ser una URL http o https válida")
	}
	if c.Interval <= 0 || c.ProcessingTime <= 0 || c.RequestTimeout <= 0 || c.BackoffBase <= 0 || c.BackoffMax <= 0 || c.ShutdownTimeout <= 0 {
		return fmt.Errorf("las duraciones deben ser mayores que cero")
	}
	if c.MaxConcurrent < 1 || c.MaxConcurrent > 20 {
		return fmt.Errorf("ORDERS_WORKER_MAX_CONCURRENT debe estar entre 1 y 20")
	}
	if c.BackoffMax < c.BackoffBase {
		return fmt.Errorf("el backoff máximo no puede ser menor que el base")
	}
	if c.HealthAddr == "" {
		return fmt.Errorf("health address obligatorio")
	}
	return nil
}
func get(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s inválido: %w", name, err)
	}
	return value, nil
}
