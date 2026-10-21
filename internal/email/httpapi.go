package email

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// callAPI runs req against a provider's HTTP API and returns the response body and headers, or an *HTTPError for a non-2xx answer.
func callAPI(provider string, req *http.Request) ([]byte, http.Header, error) {
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("email: send via %s: %w", provider, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, &HTTPError{Provider: provider, Status: resp.StatusCode, Body: string(body)}
	}
	return body, resp.Header, nil
}

func requireAPI(cfg Config, name string) (*APIConfig, error) {
	if cfg.API == nil {
		return nil, fmt.Errorf("email: %s backend selected with no API config", name)
	}
	return cfg.API, nil
}
