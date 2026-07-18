package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Address     string          `json:"address"`
	LogDir      string          `json:"log_dir"`
	GracePeriod Duration        `json:"grace_period"`
	Backoff     BackoffConfig   `json:"backoff"`
	Processes   []ProcessConfig `json:"processes"`
}

type BackoffConfig struct {
	Base       Duration `json:"base"`
	Factor     float64  `json:"factor"`
	Max        Duration `json:"max"`
	MaxRetries int      `json:"max_retries"`
}

type ProcessConfig struct {
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Restart    string   `json:"restart"`
	WorkDir    string   `json:"workdir"`
	MaxRetries *int     `json:"max_retries,omitempty"`
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("la duración debe ser texto: %w", err)
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("duración %q inválida: %w", value, err)
	}
	d.Duration = parsed
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("JSON inválido en %s: %w", path, err)
	}
	cfg.defaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuración inválida: %w", err)
	}
	return &cfg, nil
}

func (c *Config) defaults() {
	if c.Address == "" {
		c.Address = ":8080"
	}
	if c.LogDir == "" {
		c.LogDir = "logs"
	}
	if c.GracePeriod.Duration == 0 {
		c.GracePeriod.Duration = 3 * time.Second
	}
	if c.Backoff.Base.Duration == 0 {
		c.Backoff.Base.Duration = time.Second
	}
	if c.Backoff.Factor == 0 {
		c.Backoff.Factor = 2
	}
	if c.Backoff.Max.Duration == 0 {
		c.Backoff.Max.Duration = 30 * time.Second
	}
	if c.Backoff.MaxRetries == 0 {
		c.Backoff.MaxRetries = 5
	}
	for i := range c.Processes {
		if c.Processes[i].Restart == "" {
			c.Processes[i].Restart = "never"
		}
		if c.Processes[i].WorkDir == "" {
			c.Processes[i].WorkDir = "."
		}
	}
}

func (c Config) Validate() error {
	if c.Backoff.Base.Duration <= 0 || c.Backoff.Max.Duration <= 0 || c.Backoff.Factor < 1 || c.Backoff.MaxRetries < 0 {
		return fmt.Errorf("parámetros de backoff no válidos")
	}
	seen := make(map[string]bool)
	for _, p := range c.Processes {
		if p.Name == "" || strings.ContainsAny(p.Name, `/\\`) || p.Name == "." || p.Name == ".." {
			return fmt.Errorf("nombre de proceso %q no válido", p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("proceso duplicado %q", p.Name)
		}
		seen[p.Name] = true
		if strings.TrimSpace(p.Command) == "" {
			return fmt.Errorf("el proceso %q no tiene comando", p.Name)
		}
		if p.Restart != "always" && p.Restart != "on-failure" && p.Restart != "never" {
			return fmt.Errorf("política %q no válida para %q", p.Restart, p.Name)
		}
	}
	return nil
}
