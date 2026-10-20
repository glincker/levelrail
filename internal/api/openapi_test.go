package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleOpenAPISpec_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleOpenAPISpec_ListsRoutesWithExamples(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/openapi.json", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got openAPISpec
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Count == 0 || got.Count != len(got.Routes) {
		t.Fatalf("Count = %d, len(Routes) = %d, want matching non-zero counts", got.Count, len(got.Routes))
	}
	if got.ExampleCount == 0 {
		t.Error("ExampleCount = 0, want at least the hand-written examples")
	}

	var foundSelf, foundExample bool
	for _, r := range got.Routes {
		if r.Method == "GET" && r.Path == "/api/v1/openapi.json" {
			foundSelf = true
			if r.Ability != "AbilityRead" {
				t.Errorf("GET /api/v1/openapi.json ability = %q, want AbilityRead", r.Ability)
			}
		}
		if r.Method == "GET" && r.Path == "/api/v1/apps" {
			foundExample = true
			if len(r.ResponseBody) == 0 {
				t.Error("GET /api/v1/apps has no example response body, want one from openAPIExamples")
			}
		}
	}
	if !foundSelf {
		t.Error("openapi.json's own route is missing from its own spec")
	}
	if !foundExample {
		t.Error("GET /api/v1/apps missing from spec")
	}
}
