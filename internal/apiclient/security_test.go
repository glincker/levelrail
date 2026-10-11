package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityClientRoutes(t *testing.T) {
	var gotMethod, gotURI string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotURI, gotBody = r.Method, r.URL.RequestURI(), nil
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"score":80,"grade":"B","revoked":2,"approval_scope":"all_methods"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "test-token")
	ctx := context.Background()
	scope := "all_methods"
	tests := []struct {
		name, method, uri string
		call              func() error
	}{
		{"posture", http.MethodGet, "/api/v1/security/posture", func() error { _, err := c.GetSecurityPosture(ctx); return err }},
		{"policy get", http.MethodGet, "/api/v1/security/policy", func() error { _, err := c.GetSecurityPolicy(ctx); return err }},
		{"policy set", http.MethodPut, "/api/v1/security/policy", func() error {
			_, err := c.UpdateSecurityPolicy(ctx, SecurityPolicyUpdate{ApprovalScope: &scope})
			return err
		}},
		{"own sessions", http.MethodGet, "/api/v1/security/sessions", func() error { _, err := c.ListSecuritySessions(ctx, ""); return err }},
		{"admin sessions", http.MethodGet, "/api/v1/security/sessions?user_id=u+1", func() error { _, err := c.ListSecuritySessions(ctx, "u 1"); return err }},
		{"revoke", http.MethodDelete, "/api/v1/security/sessions/s1", func() error { return c.RevokeSecuritySession(ctx, "s1", "") }},
		{"revoke others", http.MethodPost, "/api/v1/security/sessions/revoke-others", func() error {
			_, err := c.RevokeOtherSecuritySessions(ctx, "")
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatal(err)
			}
			if gotMethod != tt.method || gotURI != tt.uri {
				t.Fatalf("got %s %s, want %s %s", gotMethod, gotURI, tt.method, tt.uri)
			}
		})
	}
	if _, err := c.UpdateSecurityPolicy(ctx, SecurityPolicyUpdate{ApprovalScope: &scope}); err != nil {
		t.Fatal(err)
	}
	if len(gotBody) != 1 || gotBody["approval_scope"] != scope {
		t.Fatalf("unset policy fields must be omitted: %v", gotBody)
	}
}
