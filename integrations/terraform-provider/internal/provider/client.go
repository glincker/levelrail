package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a minimal JSON client for the control plane REST API.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient returns a client for the control plane at baseURL.
func NewClient(baseURL, token string) *Client {
	return &Client{
		base:  strings.TrimRight(baseURL, "/"),
		token: token,
		http:  &http.Client{Timeout: 60 * time.Second},
	}
}

// APIError is a non-2xx answer from the control plane.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Message) }

func isNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

func esc(s string) string { return url.PathEscape(s) }

// do sends one request. body and out are JSON-encoded values of any shape
// (the API has no single response type), either may be nil.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{Status: resp.StatusCode, Message: errorMessage(data, resp.Status)}
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

func errorMessage(data []byte, fallback string) string {
	var e struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil {
		if e.Message != "" {
			return e.Message
		}
		switch v := e.Error.(type) {
		case string:
			return v
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				return m
			}
		}
	}
	return fallback
}

type wireProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type wireEnvironment struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

type wireDatabase struct {
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Version   string `json:"version"`
	ProjectID string `json:"project_id"`
}

type wireApp struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Port        int64             `json:"port"`
	BindAddress string            `json:"bind_address"`
	Strategy    string            `json:"strategy"`
	Replicas    int64             `json:"replicas"`
	Domains     []string          `json:"domains"`
	Env         map[string]string `json:"env"`
	ProjectID   string            `json:"project_id"`
}

type wireNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Status      string `json:"status"`
	Region      string `json:"region"`
	Schedulable bool   `json:"schedulable"`
}

// envPath maps an env var scope to its endpoint.
func envPath(scope, id string) (string, error) {
	switch scope {
	case "project":
		return "/api/v1/projects/" + esc(id) + "/env", nil
	case "environment":
		return "/api/v1/environments/" + esc(id) + "/env", nil
	}
	return "", fmt.Errorf("scope must be \"project\" or \"environment\", got %q", scope)
}

func (c *Client) getApp(ctx context.Context, name string) (wireApp, map[string]any, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(name), nil, &raw); err != nil {
		return wireApp{}, nil, err
	}
	var app wireApp
	buf, _ := json.Marshal(raw)
	if err := json.Unmarshal(buf, &app); err != nil {
		return wireApp{}, nil, fmt.Errorf("decode app %s: %w", name, err)
	}
	return app, raw, nil
}

// putApp saves a full app body, which the API replaces wholesale.
func (c *Client) putApp(ctx context.Context, name string, body map[string]any) error {
	return c.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(name), body, nil)
}

func (c *Client) listEnvironments(ctx context.Context, projectID string) ([]wireEnvironment, error) {
	var envs []wireEnvironment
	err := c.do(ctx, http.MethodGet, "/api/v1/projects/"+esc(projectID)+"/environments", nil, &envs)
	return envs, err
}
