package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const healthcheckTimeout = 3 * time.Second

// Used as a Docker HEALTHCHECK on the distroless image, which has no shell, curl, or wget.
func runHealthcheck(ctx context.Context, out io.Writer, ready bool) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	addr := dashboardDialAddr(httpAddr())
	if addr == "" {
		return fmt.Errorf("no dialable address for %q", httpAddr())
	}

	path := "/api/v1/brand"
	if ready {
		path = "/readyz"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil) // NOSONAR: addr is always loopback, this process checking its own local HTTP listener, never a real network HTTPS gap
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unhealthy: %s returned %d", addr, resp.StatusCode)
	}
	_, _ = fmt.Fprintln(out, "ok")
	return nil
}
