package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ORDERS_WEB_ADDR", "")
	t.Setenv("ORDERS_API_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error inesperado: %v", err)
	}
	if cfg.WebAddr != defaultWebAddr {
		t.Errorf("WebAddr = %q, se esperaba %q", cfg.WebAddr, defaultWebAddr)
	}
	if cfg.OrdersAPIURL != defaultAPIURL {
		t.Errorf("OrdersAPIURL = %q, se esperaba %q", cfg.OrdersAPIURL, defaultAPIURL)
	}
	if cfg.ClientTimeout <= 0 || cfg.ShutdownGrace <= 0 || cfg.DashboardRefresh <= 0 {
		t.Errorf("los timeouts por defecto deben ser positivos: %+v", cfg)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("ORDERS_WEB_ADDR", ":9999")
	t.Setenv("ORDERS_API_URL", "https://example.com:9091")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error inesperado: %v", err)
	}
	if cfg.WebAddr != ":9999" {
		t.Errorf("WebAddr = %q, se esperaba :9999", cfg.WebAddr)
	}
	if cfg.OrdersAPIURL != "https://example.com:9091" {
		t.Errorf("OrdersAPIURL = %q, se esperaba https://example.com:9091", cfg.OrdersAPIURL)
	}
}

func TestValidateRejectsInvalidScheme(t *testing.T) {
	cfg := Config{WebAddr: ":9092", OrdersAPIURL: "ftp://localhost:9091"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("se esperaba error para esquema ftp")
	}
}

func TestValidateRejectsMalformedURL(t *testing.T) {
	cfg := Config{WebAddr: ":9092", OrdersAPIURL: "://no-es-una-url"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("se esperaba error para URL malformada")
	}
}

func TestValidateRejectsEmptyHost(t *testing.T) {
	cfg := Config{WebAddr: ":9092", OrdersAPIURL: "http://"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("se esperaba error para host vacío")
	}
}

func TestValidateRejectsEmptyWebAddr(t *testing.T) {
	cfg := Config{WebAddr: "", OrdersAPIURL: defaultAPIURL}
	if err := cfg.Validate(); err == nil {
		t.Fatal("se esperaba error para WebAddr vacío")
	}
}
