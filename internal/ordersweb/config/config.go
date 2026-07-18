// Package config define la configuración de orders-web: dirección propia,
// URL de orders-api y los tiempos que gobiernan cliente HTTP, apagado y
// polling del panel. No usa variables globales; Load devuelve un valor
// inmutable que el composition root (cmd/orders-web/main.go) pasa explícitamente.
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"
)

const (
	defaultWebAddr        = ":9092"
	defaultAPIURL         = "http://localhost:9091"
	defaultClientTimeout  = 5 * time.Second
	defaultShutdownGrace  = 5 * time.Second
	defaultRefreshSeconds = 5 * time.Second
)

// Config agrupa los parámetros de arranque de orders-web.
type Config struct {
	// WebAddr es la dirección donde escucha el servidor de orders-web (ej. ":9092").
	WebAddr string
	// OrdersAPIURL es la base URL de orders-api (ej. "http://localhost:9091").
	OrdersAPIURL string
	// ClientTimeout limita cada solicitud del cliente HTTP hacia orders-api.
	ClientTimeout time.Duration
	// ShutdownGrace es el tiempo máximo para drenar solicitudes activas al apagar.
	ShutdownGrace time.Duration
	// DashboardRefresh es el intervalo sugerido al frontend para el polling.
	DashboardRefresh time.Duration
}

// Load construye la configuración a partir de variables de entorno, aplicando
// valores por defecto cuando faltan y validando que ORDERS_API_URL sea una URL
// http o https bien formada.
func Load() (Config, error) {
	cfg := Config{
		WebAddr:          envOr("ORDERS_WEB_ADDR", defaultWebAddr),
		OrdersAPIURL:     envOr("ORDERS_API_URL", defaultAPIURL),
		ClientTimeout:    defaultClientTimeout,
		ShutdownGrace:    defaultShutdownGrace,
		DashboardRefresh: defaultRefreshSeconds,
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate comprueba que la configuración sea utilizable.
func (c Config) Validate() error {
	if c.WebAddr == "" {
		return fmt.Errorf("ORDERS_WEB_ADDR no puede estar vacío")
	}
	parsed, err := url.Parse(c.OrdersAPIURL)
	if err != nil {
		return fmt.Errorf("ORDERS_API_URL %q no es una URL válida: %w", c.OrdersAPIURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("ORDERS_API_URL %q debe usar esquema http o https", c.OrdersAPIURL)
	}
	if parsed.Host == "" {
		return fmt.Errorf("ORDERS_API_URL %q debe incluir un host", c.OrdersAPIURL)
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
