package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/serverready"
)

func TestServerReadiness(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/readiness", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got serverready.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Checks) == 0 || got.NextStep == "" || got.Mode == "" {
		t.Fatalf("report = %+v", got)
	}

	for _, domain := range []string{"not%20a%20host", "a%2Fb.example.com", "localhost"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/readiness?domain="+domain, ""))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("domain %q: status %d, want 400", domain, rec.Code)
		}
	}
}
