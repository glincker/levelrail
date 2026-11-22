package cloudspend

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// maxBody bounds what one provider response may hold.
const maxBody = 8 << 20

func httpClient(cfg Config) *http.Client {
	return &http.Client{Timeout: cfg.HTTPTimeout}
}

// doJSON sends one request and decodes the reply. A failure keeps only the
// status and a short type name, so provider bodies and OCIDs never travel up.
func doJSON(c *http.Client, req *http.Request, out any) (http.Header, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, &ProviderError{Code: "transport"}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, &ProviderError{Status: resp.StatusCode, Code: "read"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &ProviderError{Status: resp.StatusCode, Code: errType(raw)}
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, &ProviderError{Status: resp.StatusCode, Code: "decode"}
		}
	}
	return resp.Header, nil
}

func errType(raw []byte) string {
	var e struct {
		Type string `json:"__type"`
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	t := e.Type
	if t == "" {
		t = e.Code
	}
	for i := len(t) - 1; i >= 0; i-- {
		if t[i] == '#' {
			return t[i+1:]
		}
	}
	return t
}

func get(ctx context.Context, url string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
}

func parseTime(s string) *time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}
