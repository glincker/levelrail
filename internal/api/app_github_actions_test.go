package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleAppGitHubActions(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	tests := []struct {
		name, method, path, body string
		wantCode                 int
		wantEnv                  string
		wantChecks               bool
	}{
		{"defaults", http.MethodGet, "/api/v1/apps/web/github-actions", "", http.StatusOK, "production", true},
		{"unknown app", http.MethodGet, "/api/v1/apps/nope/github-actions", "", http.StatusNotFound, "", false},
		{"bad environment", http.MethodPut, "/api/v1/apps/web/github-actions", `{"environment":"a;b"}`, http.StatusBadRequest, "", false},
		{"save", http.MethodPut, "/api/v1/apps/web/github-actions", `{"environment":"staging","report_deployments":true,"report_checks":false}`, http.StatusOK, "staging", false},
		{"read back", http.MethodGet, "/api/v1/apps/web/github-actions", "", http.StatusOK, "staging", false},
		{"blank environment defaults", http.MethodPut, "/api/v1/apps/web/github-actions", `{"environment":" ","report_checks":true}`, http.StatusOK, "production", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tt.method, tt.path, tt.body))
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var got appGitHubActionsResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Environment != tt.wantEnv || got.ReportChecks != tt.wantChecks {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
