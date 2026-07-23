package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseSize = 2 << 20

type Client struct {
	base  string
	http  *http.Client
	token string
}

func New(base string, timeout time.Duration, tokens ...string) (*Client, error) {
	u, err := url.ParseRequestURI(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("URL de orders-api inválida")
	}
	token := ""
	if len(tokens) > 0 {
		token = tokens[0]
	}
	return &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: timeout}, token: token}, nil
}
func (c *Client) Health(ctx context.Context) error {
	var response map[string]any
	return c.do(ctx, http.MethodGet, "/health", nil, &response)
}
func (c *Client) ListOrders(ctx context.Context) ([]Order, error) {
	var response listResponse
	if err := c.do(ctx, http.MethodGet, "/api/orders", nil, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}
func (c *Client) UpdateOrderStatus(ctx context.Context, id int64, status string) error {
	return c.do(ctx, http.MethodPatch, fmt.Sprintf("/api/orders/%d/status", id), map[string]string{"status": status}, nil)
}
func (c *Client) do(ctx context.Context, method, path string, body any, result any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("codificar solicitud: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return fmt.Errorf("crear solicitud: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxResponseSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("leer respuesta: %w", err)
	}
	if len(data) > maxResponseSize {
		return ErrResponseTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return classify(resp.StatusCode)
	}
	if result != nil {
		if err := json.Unmarshal(data, result); err != nil {
			return fmt.Errorf("JSON de respuesta inválido: %w", err)
		}
	}
	return nil
}
func classify(status int) error {
	kind := ErrPermanent
	switch {
	case status == http.StatusNotFound:
		kind = ErrNotFound
	case status == http.StatusBadRequest:
		kind = ErrInvalidStatus
	case status == http.StatusTooManyRequests || status >= 500:
		kind = ErrTemporary
	}
	return &HTTPError{Status: status, Kind: kind}
}
