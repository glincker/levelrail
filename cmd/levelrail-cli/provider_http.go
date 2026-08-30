package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// providerGetJSON performs a GET against rawURL, decodes a 2xx JSON body
// into out, and turns a non-2xx response into the error returned by
// newAPIErr. Shared by CoolifyClient.do and DokployClient.do: both are a
// read-only REST client for a competitor's migration-source API, and the
// request/response handling is otherwise identical between them, only the
// URL shape, auth header, and error type differ.
func providerGetJSON(ctx context.Context, hc *http.Client, rawURL, path string, setHeaders func(*http.Request), newAPIErr func(status int, data []byte) error, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil) //nolint:gosec // rawURL is the operator-supplied --url target this command exists to call, not attacker-controlled input
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	setHeaders(req)

	resp, err := hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return fmt.Errorf("request GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIErr(resp.StatusCode, data)
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}
