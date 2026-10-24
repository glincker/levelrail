package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Cleanup(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	tests := []struct {
		name      string
		reply     string
		call      func(*Client) (any, error)
		wantVerb  string
		wantPath  string
		wantQuery string
		wantBody  string
		check     func(t *testing.T, got any)
	}{
		{
			name:     "list",
			reply:    `{"targets":[{"id":"audit_log","mode":"on","usage":{"rows":9}}],"scheduler":{"enabled":true,"interval":"6h"}}`,
			call:     func(c *Client) (any, error) { return c.GetCleanup(context.Background()) },
			wantVerb: http.MethodGet,
			wantPath: "/api/v1/cleanup",
			check: func(t *testing.T, got any) {
				l := got.(CleanupListResource)
				if len(l.Targets) != 1 || l.Targets[0].Usage.Rows != 9 || !l.Scheduler.Enabled {
					t.Errorf("list = %+v", l)
				}
			},
		},
		{
			name:     "preview without days sends empty body",
			reply:    `{"rows":4,"bytes":128}`,
			call:     func(c *Client) (any, error) { return c.PreviewCleanup(context.Background(), "audit_log", nil) },
			wantVerb: http.MethodPost,
			wantPath: "/api/v1/cleanup/audit_log/preview",
			wantBody: `{}`,
			check: func(t *testing.T, got any) {
				if got.(CleanupImpact).Rows != 4 {
					t.Errorf("preview = %+v", got)
				}
			},
		},
		{
			name:     "preview escapes id and sends days",
			reply:    `{"rows":1,"bytes":1}`,
			call:     func(c *Client) (any, error) { return c.PreviewCleanup(context.Background(), "auth.a b", intPtr(30)) },
			wantVerb: http.MethodPost,
			wantPath: "/api/v1/cleanup/auth.a b/preview",
			wantBody: `{"days":30}`,
		},
		{
			name:     "run dry",
			reply:    `{"target":"audit_log","dry_run":true,"rows":2}`,
			call:     func(c *Client) (any, error) { return c.RunCleanup(context.Background(), "audit_log", nil, true) },
			wantVerb: http.MethodPost,
			wantPath: "/api/v1/cleanup/audit_log/run",
			wantBody: `{"dry_run":true}`,
			check: func(t *testing.T, got any) {
				if r := got.(CleanupRunResult); !r.DryRun || r.Rows != 2 {
					t.Errorf("run = %+v", r)
				}
			},
		},
		{
			name:  "run with days and no dry run",
			reply: `{"target":"audit_log","days":90,"rows":5}`,
			call: func(c *Client) (any, error) {
				return c.RunCleanup(context.Background(), "audit_log", intPtr(90), false)
			},
			wantVerb: http.MethodPost,
			wantPath: "/api/v1/cleanup/audit_log/run",
			wantBody: `{"days":90}`,
		},
		{
			name:     "mode",
			reply:    `{"id":"audit_log","mode":"preview"}`,
			call:     func(c *Client) (any, error) { return c.SetCleanupMode(context.Background(), "audit_log", "preview") },
			wantVerb: http.MethodPut,
			wantPath: "/api/v1/cleanup/audit_log/mode",
			wantBody: `{"mode":"preview"}`,
		},
		{
			name:  "runs with filters",
			reply: `{"runs":[{"id":3,"target":"audit_log"}],"next_cursor":"abc"}`,
			call: func(c *Client) (any, error) {
				return c.ListCleanupRuns(context.Background(), CleanupRunsQuery{Target: "audit_log", Since: "2026-10-01T00:00:00Z"})
			},
			wantVerb:  http.MethodGet,
			wantPath:  "/api/v1/cleanup/runs",
			wantQuery: "since=2026-10-01T00%3A00%3A00Z&target=audit_log",
			check: func(t *testing.T, got any) {
				p := got.(CleanupRunsPage)
				if len(p.Runs) != 1 || p.Runs[0].ID != 3 || p.NextCursor != "abc" {
					t.Errorf("runs = %+v", p)
				}
			},
		},
		{
			name:     "runs without filters sends no query",
			reply:    `{"runs":[]}`,
			call:     func(c *Client) (any, error) { return c.ListCleanupRuns(context.Background(), CleanupRunsQuery{}) },
			wantVerb: http.MethodGet,
			wantPath: "/api/v1/cleanup/runs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotVerb, gotPath, gotQuery, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotVerb, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.reply))
			}))
			t.Cleanup(srv.Close)

			c := NewClient(srv.URL, "tok")
			got, err := tc.call(c)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if gotVerb != tc.wantVerb {
				t.Errorf("verb = %s, want %s", gotVerb, tc.wantVerb)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
			if gotQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tc.wantQuery)
			}
			if tc.wantBody != "" && gotBody != tc.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tc.wantBody)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}
