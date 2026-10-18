package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFrameworkRoutes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name   string
		path   string
		status int
	}{
		{"list", "/api/v1/frameworks", http.StatusOK},
		{"get", "/api/v1/frameworks/spring-boot", http.StatusOK},
		{"get unknown", "/api/v1/frameworks/nope", http.StatusNotFound},
		{"dockerfile", "/api/v1/frameworks/go/dockerfile", http.StatusOK},
		{"dockerfile none", "/api/v1/frameworks/django/dockerfile", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, tt.path, ""))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/frameworks", ""))
	var list frameworkListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Frameworks) < 10 {
		t.Fatalf("list decode err=%v len=%d", err, len(list.Frameworks))
	}
}
