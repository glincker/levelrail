package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const healthcheckTimeout = 3 * time.Second

// runHealthcheck GETs /api/v1/brand on the control plane's own loopback
// address and returns an error on anything but a 2xx response. It exists
// so `levelrail healthcheck` can back a Docker HEALTHCHECK/compose probe
// on the distroless image, which has no shell and no curl/wget.
func runHealthcheck(ctx context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	addr := dashboardDialAddr(httpAddr())
	if addr == "" {
		return fmt.Errorf("no dialable address for %q", httpAddr())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/v1/brand", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
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
