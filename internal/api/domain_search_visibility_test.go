package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDomainSearchVisibility_RoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	const path = "/api/v1/apps/web/domains/app.example.com/search-visibility"

	get := func() domainSearchVisibilityResource {
		t.Helper()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got domainSearchVisibilityResource
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got
	}

	if got := get(); got.Hidden {
		t.Fatalf("default = %+v, want visible", got)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, path, `{"hidden":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := get(); !got.Hidden {
		t.Fatalf("after hide = %+v, want hidden", got)
	}
	if hidden, err := db.IsDomainHidden(context.Background(), "app.example.com"); err != nil || !hidden {
		t.Fatalf("store = %v, %v, want hidden", hidden, err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, path, `{"hidden":false}`))
	if rec.Code != http.StatusOK || get().Hidden {
		t.Fatalf("unhide failed: status %d", rec.Code)
	}
}

func TestDomainSearchVisibility_Rejects(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/search-visibility", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty body status = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/other.example.com/search-visibility", `{"hidden":true}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("foreign domain status = %d, want 404", rec.Code)
	}
}
