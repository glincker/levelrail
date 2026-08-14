// Package probe implements HTTP readiness checking: the controller calls
// out to a container over HTTP, rather than the container self-reporting
// health via Docker's own HEALTHCHECK state machine. The app spec's
// health.readiness/liveness config is modeled on Kubernetes' prober
// pattern for exactly this reason, the same choice ADR 002's
// Consequences section already commits to over Coolify's confirmed
// weaker default (health checking disabled unless the user opts in,
// gated entirely on Docker's own HEALTHCHECK).
//
// This package implements the readiness half only: waiting for a freshly
// started container to become ready before an application controller
// cuts traffic to it or removes the container it's replacing. Liveness
// (continuously monitoring an already-running container and triggering a
// restart after repeated failures) is a related but genuinely separate
// capability, deliberately not built here, see TASKS.md 1.3's notes.
package probe

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Config is one HTTP probe's settings. Zero Interval or Timeout means
// "not specified" (matching internal/spec's optional YAML fields);
// WaitReady applies sensible defaults rather than busy-looping on a
// zero-length interval or never timing out a single attempt.
type Config struct {
	Path     string
	Interval time.Duration
	Timeout  time.Duration
}

const (
	defaultInterval = 2 * time.Second
	defaultTimeout  = 2 * time.Second
)

// WaitReady polls http://addr+cfg.Path on cfg.Interval, each attempt
// bounded by cfg.Timeout, until a response in the 2xx range comes back
// or ctx is done. It returns nil on the first success, or ctx's error
// (wrapped with the last probe failure reason, if any) once ctx expires.
//
// client is caller-supplied rather than using http.DefaultClient, so
// callers (and tests) control transport settings and can inject a client
// pointed at a test server.
func WaitReady(ctx context.Context, client *http.Client, addr string, cfg Config) error {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	url := "http://" + addr + cfg.Path

	var lastErr error
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		lastErr = attempt(ctx, client, url, timeout)
		if lastErr == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("probe: %s never became ready: %w (last attempt: %v)", url, ctx.Err(), lastErr)
		case <-ticker.C:
			// try again
		}
	}
}

func attempt(ctx context.Context, client *http.Client, url string, timeout time.Duration) error {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
