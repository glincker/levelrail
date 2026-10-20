package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newFixitRouter(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithFixit(db))
	rt.fixit.host = func() fixit.HostEnv { return fixit.HostEnv{Mode: fixit.RunSystemd, Unit: "lr", TunPresent: true} }
	return rt, db
}

func TestFixesListPreviewApply(t *testing.T) {
	rt, db := newFixitRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/fixes?subject=mesh&all=1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", rec.Code, rec.Body)
	}
	var list struct {
		Fixes []fixit.View `json:"fixes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Fixes) == 0 {
		t.Fatalf("list body: %v %s", err, rec.Body)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/nope/preview", "{}"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown fix status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/mesh.public_address.set/preview", "{}"))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status %d: %s", rec.Code, rec.Body)
	}
}

func TestAdvertiseHostOverrideViaApply(t *testing.T) {
	rt, db := newFixitRouter(t)
	rt.doctorAgentAdvertiseHost = "127.0.0.1"
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/mesh.public_address.set/apply", `{"params":{"host":"203.0.113.9"}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body)
	}
	var res fixit.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Outcome != fixit.OutcomeFixed {
		t.Fatalf("want fixed, got %+v err=%v", res, err)
	}
	if got := rt.advertiseHost(); got != "203.0.113.9" {
		t.Fatalf("advertiseHost = %q", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/mesh.public_address.set/apply", `{"params":{"host":"127.0.0.1"}}`))
	var again fixit.Result
	_ = json.Unmarshal(rec.Body.Bytes(), &again)
	if again.Outcome != fixit.OutcomeSkipped {
		t.Fatalf("healthy check must skip, got %+v", again)
	}
}

func TestFixesNotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/fixes", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status %d", rec.Code)
	}
}
