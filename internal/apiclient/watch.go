package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WatchEvent mirrors internal/api's own watchEvent (app_watch.go): the
// JSON payload WatchApp/WatchDatabase deliver to onEvent each time the
// polled reconcile conditions change.
type WatchEvent struct {
	Conditions []ConditionResource `json:"conditions"`
	ObservedAt time.Time           `json:"observed_at"`
}

// WatchApp opens GET /api/v1/apps/{name}/watch and calls onEvent once
// per SSE event received, blocking until ctx is cancelled or the server
// ends the connection. Cancelling ctx is the only supported way to stop
// early; onEvent has no way to signal "stop" itself.
func (c *Client) WatchApp(ctx context.Context, name string, onEvent func(WatchEvent)) error {
	return c.watch(ctx, "/api/v1/apps/"+PathEscape(name)+"/watch", onEvent)
}

// WatchDatabase is WatchApp's database analogue: GET
// /api/v1/databases/{name}/watch.
func (c *Client) WatchDatabase(ctx context.Context, name string, onEvent func(WatchEvent)) error {
	return c.watch(ctx, "/api/v1/databases/"+PathEscape(name)+"/watch", onEvent)
}

// watch is the shared SSE-consuming loop WatchApp/WatchDatabase both use:
// same "data: " line convention internal/api/deploy_attempts.go's
// writeSSEEvent produces server-side, skipping the leading ": connected"
// comment line and any blank event-separator lines.
//
// It builds its own *http.Client rather than reusing c.hc: that one
// carries a fixed 15-minute Timeout (sized for a long build, see
// NewClient's own doc comment), which is a request-lifetime cap that
// would silently kill a watch left running longer than that. A watch's
// lifetime is meant to be bounded by ctx (the caller hitting Ctrl-C)
// alone, so this client has no Timeout and shares c.hc's Transport for
// connection reuse.
func (c *Client) watch(ctx context.Context, path string, onEvent func(WatchEvent)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil) //nolint:gosec // c.baseURL is the operator-supplied API target this client exists to call, not attacker-controlled input
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	streamClient := &http.Client{Transport: c.hc.Transport}
	resp, err := streamClient.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return fmt.Errorf("request GET %s: %w", c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Message: ExtractErrorMessage(data), RetryAfter: retryAfterHeader(resp.Header)}
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			if readErr == io.EOF {
				return nil
			}
			return fmt.Errorf("read event stream: %w", readErr)
		}
		data, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "data: ")
		if !ok {
			continue
		}
		var ev WatchEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		onEvent(ev)
	}
}
