package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCreateStaticSite(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	do := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites", body))
		return rec
	}

	rec := do(`{"name":"my-pages"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var got staticSiteDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "my-pages" || got.BuildType != "static" {
		t.Errorf("unexpected body: %+v", got)
	}
	if rec := do(`{"name":"my-pages"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate = %d, want 409", rec.Code)
	}
	if rec := do(`{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty name = %d, want 400", rec.Code)
	}
}
