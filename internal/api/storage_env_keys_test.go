package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
)

func TestHandleListStorageEnvKeys(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/storage-env-keys", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Asserts equality with the real exported constant, not a
	// hand-copied literal list here: this is the whole point of the
	// endpoint, a caller (this test included) should never need its own
	// separate copy of these names to check against.
	if len(got) != len(application.StorageEnvKeys) {
		t.Fatalf("got %d keys, want %d matching application.StorageEnvKeys", len(got), len(application.StorageEnvKeys))
	}
	for i, want := range application.StorageEnvKeys {
		if got[i] != want {
			t.Errorf("key[%d] = %q, want %q", i, got[i], want)
		}
	}
}

func TestStorageEnvKeysRoute_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/storage-env-keys", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}
