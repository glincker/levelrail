package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestListEndpointsNeverReturnNullOnEmptyStore walks every parameterless
// GET route on an empty store and fails when a response body is a bare
// JSON null or an object with a top-level null field. The dashboard calls
// .length and .map on these, so a Go nil slice must never reach it.
func TestListEndpointsNeverReturnNullOnEmptyStore(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	h := rt.Handler()

	for _, route := range openAPIRoutes {
		if route.Method != http.MethodGet || strings.Contains(route.Path, "{") || strings.Contains(route.Path, "/stream") {
			continue
		}
		rec := httptest.NewRecorder()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req := authedRequest(t, cookie, http.MethodGet, route.Path, "").WithContext(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = recover() }()
			h.ServeHTTP(rec, req)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// Long-lived handler: not a list endpoint, skip without reading rec.
			cancel()
			continue
		}
		cancel()
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			continue
		}
		body := strings.TrimSpace(rec.Body.String())
		if body == "null" {
			t.Errorf("GET %s returned a bare null (%s)", route.Path, route.Handler)
			continue
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal([]byte(body), &obj) != nil {
			continue
		}
		for k, v := range obj {
			if string(v) == "null" && !nullableFieldAllowlist[route.Path+" "+k] {
				t.Errorf("GET %s field %q is null (%s)", route.Path, k, route.Handler)
			}
		}
	}
}

// nullableFieldAllowlist lists "path field" pairs where null is a real
// value (an unset pointer), not a nil slice. Keep it short.
var nullableFieldAllowlist = map[string]bool{
	"/api/v1/auth/login-options sso_route":                   true,
	"/api/v1/deployments/summary failure_rate_24h":           true,
	"/api/v1/git/repos next_offset":                          true,
	"/api/v1/settings/ingress/edge idle_timeout_secs":        true,
	"/api/v1/settings/ingress/edge max_body_bytes":           true,
	"/api/v1/settings/ingress/edge max_header_bytes":         true,
	"/api/v1/settings/ingress/edge read_header_timeout_secs": true,
	"/api/v1/me/onboarding started_at":                       true,
	"/api/v1/me/onboarding dismissed_at":                     true,
	"/api/v1/me/onboarding completed_at":                     true,
	"/api/v1/me/preferences/appearance value":                true,
	"/api/v1/migration/takeover/plan run":                    true,
	"/api/v1/updates/preflight migrations_to_apply":          true,
	"/api/v1/updates/preflight latest_schema":                true,
	"/api/v1/updates/preflight db_schema":                    true,
	"/api/v1/updates/releases current_schema_version":        true,
	"/api/v1/updates/schedule active":                        true,
	"/api/v1/updates/schedule next_window":                   true,
	"/api/v1/updates/self-upgrade/status progress":           true,
	"/api/v1/updates/self-upgrade/status running":            true,
	"/api/v1/updates/self-upgrade/status last_finished":      true,
}

type nullShapeInner struct {
	Tags []string          `json:"tags"`
	Meta map[string]string `json:"meta"`
}

type nullShapeOuter struct {
	Items   []nullShapeInner `json:"items"`
	Nested  *nullShapeInner  `json:"nested"`
	Skipped []string         `json:"skipped,omitempty"`
	Ptr     *int             `json:"ptr"`
	Any     map[string]any   `json:"any"`
}

func TestWriteJSONEmptiesNilSlicesAndMaps(t *testing.T) {
	t.Parallel()
	cases := map[string]any{
		"top level slice": []string(nil),
		"struct":          nullShapeOuter{Items: []nullShapeInner{{}}, Nested: &nullShapeInner{}, Any: map[string]any{"k": []int(nil)}},
		"pointer":         &nullShapeOuter{},
	}
	for name, v := range cases {
		rec := httptest.NewRecorder()
		writeJSON(rec, http.StatusOK, v)
		body := rec.Body.String()
		for _, bad := range []string{`"tags":null`, `"meta":null`, `"items":null`, `"any":null`, `"k":null`} {
			if strings.Contains(body, bad) {
				t.Errorf("%s: body %s contains %s", name, body, bad)
			}
		}
		if strings.TrimSpace(body) == "null" {
			t.Errorf("%s: bare null", name)
		}
		if strings.Contains(body, `"skipped"`) {
			t.Errorf("%s: omitempty field was emitted: %s", name, body)
		}
		if name != "top level slice" && !strings.Contains(body, `"ptr":null`) {
			t.Errorf("%s: nil pointer should stay null: %s", name, body)
		}
	}
}
