package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glincker/levelrail/internal/experimental"
)

func TestHandleListDatabaseEngines(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/database-engines", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}

// Not parallel: the experimental gate is process global.
func TestDocumentDBHiddenWhileFlagOff(t *testing.T) {
	// Reset would leave every flag off for later tests; Set() is off now and restored after.
	experimental.Set()
	t.Cleanup(enableAllExperimental)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/database-engines", ""))
	var got []databaseEngineResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, e := range got {
		if e.ID == "documentdb" {
			t.Fatal("documentdb listed while the beta feature is off")
		}
	}
	err := validateDatabaseResource(databaseResource{Name: "docs", Engine: "documentdb", Version: "pg17-1.2.0"})
	if err == nil {
		t.Fatal("create must be rejected while the beta feature is off")
	}
}
