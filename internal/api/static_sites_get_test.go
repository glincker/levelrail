package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGetStaticSite(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	root := t.TempDir()
	rt.staticRootDir = root
	seedApp(t, db, "live")
	seedStaticSite(t, db, root, "live", "live.example.com")
	seedApp(t, db, "fresh")

	get := func(name string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/"+name, ""))
		return rec
	}

	rec := get("live")
	if rec.Code != http.StatusOK {
		t.Fatalf("get live = %d %s", rec.Code, rec.Body.String())
	}
	var got staticSiteDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Published || got.FileCount != 1 || got.SizeBytes != 2 || len(got.Domains) != 1 {
		t.Errorf("unexpected detail: %+v", got)
	}

	rec = get("fresh")
	if rec.Code != http.StatusOK {
		t.Fatalf("get fresh = %d %s", rec.Code, rec.Body.String())
	}
	got = staticSiteDetail{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Published {
		t.Errorf("fresh site should be unpublished: %+v err=%v", got, err)
	}

	if rec := get("missing"); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
}
