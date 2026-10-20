package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateSpecRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/apps/web/validate-spec"},
	})
}

func TestHandleValidateSpec(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantValid  bool
		wantIssue  string
		wantIssues int
	}{
		{
			name:      "valid minimal spec",
			body:      `{"yaml":"version: 1\nservices:\n  web:\n    build: {type: image, image: nginx:latest}\n    port: 80\n"}`,
			wantValid: true,
		},
		{
			name:       "schema violation reports path and line",
			body:       `{"yaml":"version: 1\nservices:\n  web:\n    build: {type: bogus}\n"}`,
			wantValid:  false,
			wantIssue:  "services.web.build.type",
			wantIssues: 1,
		},
		{
			name:      "semantic error still a structured issue",
			body:      `{"yaml":"version: 1\nservices:\n  web:\n    build: {type: static}\n    port: 80\n"}`,
			wantValid: false,
			wantIssue: "services.web",
		},
		{
			name:      "invalid yaml",
			body:      `{"yaml":"services: [not a map"}`,
			wantValid: false,
		},
		{
			name:      "invalid json body",
			body:      `not json`,
			wantValid: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			seedWebAppForTest(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/validate-spec", tt.body))

			if tt.name == "invalid json body" {
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", rec.Code)
				}
				return
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			var resp validateSpecResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.Valid != tt.wantValid {
				t.Fatalf("valid = %v, want %v (issues: %+v)", resp.Valid, tt.wantValid, resp.Issues)
			}
			if tt.wantIssues > 0 && len(resp.Issues) != tt.wantIssues {
				t.Fatalf("len(issues) = %d, want %d: %+v", len(resp.Issues), tt.wantIssues, resp.Issues)
			}
			if tt.wantIssue != "" {
				found := false
				for _, issue := range resp.Issues {
					if issue.Path == tt.wantIssue {
						found = true
					}
				}
				if !found {
					t.Fatalf("issues = %+v, want one with path %q", resp.Issues, tt.wantIssue)
				}
			}
		})
	}
}

func TestHandleValidateSpec_UnknownAppStillValidates(t *testing.T) {
	// Stateless: validate-spec never looks the app up, so a name that
	// doesn't exist yet (the app-creation flow, before a real app
	// resource exists) still gets a real answer, not a 404.
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"yaml":"version: 1\nservices:\n  web:\n    build: {type: image, image: nginx:latest}\n    port: 80\n"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/does-not-exist/validate-spec", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"valid"`) {
		t.Fatalf("body = %s, want a validation response", rec.Body.String())
	}
}
