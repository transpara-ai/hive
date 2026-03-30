package membrane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ServiceClient abstracts communication with a wrapped external service.
type ServiceClient interface {
	Get(ctx context.Context, path string) (json.RawMessage, error)
	Post(ctx context.Context, path string, body interface{}) (json.RawMessage, error)
}

// HTTPServiceClient implements ServiceClient over HTTP.
type HTTPServiceClient struct {
	BaseURL    string
	AuthMethod string
	AuthConfig map[string]string
	Timeout    time.Duration
	client     *http.Client
}

// NewHTTPServiceClient creates a client for the given service endpoint.
func NewHTTPServiceClient(baseURL, authMethod string, authConfig map[string]string) *HTTPServiceClient {
	return &HTTPServiceClient{
		BaseURL:    baseURL,
		AuthMethod: authMethod,
		AuthConfig: authConfig,
		Timeout:    30 * time.Second,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HTTPServiceClient) do(req *http.Request) (json.RawMessage, error) {
	c.applyAuth(req)
	if c.Timeout > 0 {
		c.client.Timeout = c.Timeout
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("service request %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("service %s %s returned %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}

	return json.RawMessage(body), nil
}

// Get performs a GET request to the service.
func (c *HTTPServiceClient) Get(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

// Post performs a POST request to the service.
func (c *HTTPServiceClient) Post(ctx context.Context, path string, body interface{}) (json.RawMessage, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *HTTPServiceClient) applyAuth(req *http.Request) {
	switch c.AuthMethod {
	case "api_key":
		req.Header.Set("X-Api-Key", c.AuthConfig["key"])
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.AuthConfig["token"])
	case "oauth":
		req.Header.Set("Authorization", "Bearer "+c.AuthConfig["access_token"])
	}
}
