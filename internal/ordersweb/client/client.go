// Package client implementa un cliente HTTP hacia orders-api. Es la única
// fuente de datos de orders-web: no hay almacenamiento propio de pedidos.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// maxResponseBody limita cuánto se lee de una respuesta de orders-api para
// evitar cargar en memoria una respuesta arbitrariamente grande.
const maxResponseBody = 5 << 20 // 5 MiB

// Client es un cliente HTTP reutilizable y sin estado mutable compartido más
// allá de lo que ya ofrece http.Client (seguro para uso concurrente).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New crea un Client apuntando a baseURL con el timeout indicado. Valida que
// baseURL sea una URL http/https bien formada; nunca usa http.DefaultClient
// para poder fijar su propio timeout.
func New(baseURL string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("baseURL %q no es válida: %w", baseURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("baseURL %q debe usar esquema http o https", baseURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("baseURL %q debe incluir un host", baseURL)
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// do ejecuta una solicitud HTTP hacia orders-api y decodifica el cuerpo JSON
// de éxito en out (si out no es nil). Distingue tres clases de fallo:
//   - error de conectividad (ErrUnavailable / ErrTimeout / ErrBadGateway);
//   - *APIError, cuando orders-api sí respondió con un error JSON válido;
//   - nil, cuando la respuesta fue 2xx y se decodificó correctamente.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("codificar cuerpo de solicitud: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("construir solicitud: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
			return fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBody+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("%w: leer respuesta: %v", ErrBadGateway, err)
	}
	if len(data) > maxResponseBody {
		return fmt.Errorf("%w: respuesta demasiado grande", ErrBadGateway)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%w: cuerpo JSON inválido: %v", ErrBadGateway, err)
	}
	return nil
}

func decodeAPIError(status int, data []byte) error {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Error == "" {
		return fmt.Errorf("%w: código %d sin cuerpo de error reconocible", ErrBadGateway, status)
	}
	return &APIError{StatusCode: status, Code: payload.Error, Message: payload.Message}
}
