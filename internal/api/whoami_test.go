package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleWhoami(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	exp := time.Now().Add(time.Hour)
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok-who", Name: "ci-token", TokenHash: hashToken("who-secret"), Abilities: []string{AbilityDeploy}, ExpiresAt: &exp,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	tests := []struct {
		name     string
		req      func() *http.Request
		wantCode int
		wantKind string
		wantName string
	}{
		{name: "session", req: func() *http.Request { return authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/whoami", "") }, wantCode: 200, wantKind: "session", wantName: testAdminUsername},
		{name: "token without read ability", req: func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
			r.Header.Set("Authorization", "Bearer who-secret")
			return r
		}, wantCode: 200, wantKind: "token", wantName: "ci-token"},
		{name: "no auth", req: func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil) }, wantCode: 401},
		{name: "bad token", req: func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
			r.Header.Set("Authorization", "Bearer nope")
			return r
		}, wantCode: 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, tt.req())
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != 200 {
				return
			}
			var got whoamiResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Kind != tt.wantKind || got.Name != tt.wantName {
				t.Errorf("got kind=%q name=%q, want %q %q", got.Kind, got.Name, tt.wantKind, tt.wantName)
			}
			if got.ExpiresAt == "" {
				t.Errorf("expires_at empty, want RFC3339")
			}
		})
	}
}
