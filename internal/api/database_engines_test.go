package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListDatabaseEngines(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/database-engines", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []databaseEngineResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("got 0 engines, want the real embedded registry's contents")
	}

	byID := make(map[string]databaseEngineResource, len(got))
	for _, e := range got {
		if e.Label == "" {
			t.Errorf("engine %q has an empty label", e.ID)
		}
		if e.DefaultVersion == "" {
			t.Errorf("engine %q has an empty default_version", e.ID)
		}
		byID[e.ID] = e
	}
	for _, want := range []string{"postgres", "redis", "mysql", "mongodb", "mariadb", "keydb"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("response missing expected engine %q", want)
		}
	}
}

func TestDatabaseEnginesRoute_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/database-engines", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}
