package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// httpSinkTimeout bounds one Send call: a generic collector endpoint an
// operator points this at is out of this control plane's control, and a
// stuck HTTP response must never pile up forwarder goroutines.
const httpSinkTimeout = 10 * time.Second

// httpSinkLine is one LogEntry's wire shape on an HTTPSink POST body,
// deliberately its own type rather than reusing LogEntry directly: an
// external collector is a stable public-ish contract, so its field names
// and shapes should change only on purpose, not as a side effect of an
// unrelated internal LogEntry refactor.
type httpSinkLine struct {
	ResourceID string `json:"resource_id"`
	Stream     string `json:"stream"`
	Timestamp  string `json:"timestamp"`
	Message    string `json:"message"`
	Structured bool   `json:"structured,omitempty"`
	FieldsJSON string `json:"fields_json,omitempty"`
}

// HTTPSink POSTs a JSON array of httpSinkLine to URL for every Send
// call, the generic-webhook log drain shape (Coolify-parity item, see
// this package's drain.go doc comment).
type HTTPSink struct {
	URL    string
	Client *http.Client
}

// NewHTTPSink builds an HTTPSink. client defaults to a fresh
// *http.Client with httpSinkTimeout if nil.
func NewHTTPSink(url string, client *http.Client) *HTTPSink {
	if client == nil {
		client = &http.Client{Timeout: httpSinkTimeout}
	}
	return &HTTPSink{URL: url, Client: client}
}

// Send implements DrainSink.
func (s *HTTPSink) Send(ctx context.Context, entries []LogEntry) error {
	lines := make([]httpSinkLine, len(entries))
	for i, e := range entries {
		lines[i] = httpSinkLine{
			ResourceID: e.ResourceID,
			Stream:     e.Stream,
			Timestamp:  e.Timestamp.UTC().Format(time.RFC3339Nano),
			Message:    e.Message,
			Structured: e.Structured,
			FieldsJSON: e.FieldsJSON,
		}
	}

	body, err := json.Marshal(lines)
	if err != nil {
		return fmt.Errorf("telemetry: encode http drain payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telemetry: build http drain request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("telemetry: http drain request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telemetry: http drain receiver returned status %d", resp.StatusCode)
	}
	return nil
}
