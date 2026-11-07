package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func createTagForRename(t *testing.T, rt *Router, cookie *http.Cookie, name string) tagResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/tags", `{"name":"`+name+`"}`))
	var out tagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("create %q: status %d body %s", name, rec.Code, rec.Body.String())
	}
	return out
}

func TestHandleRenameTag_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tag := createTagForRename(t, rt, cookie, "production")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPatch, "/api/v1/tags/"+tag.ID, `{"name":"prod"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != tag.ID || got.Name != "prod" {
		t.Errorf("renamed = %+v, want same id and name prod", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/tags", ""))
	var list []tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].AppCount == nil || *list[0].AppCount != 0 || list[0].ResourceCount == nil {
		t.Errorf("list = %+v, want one tag carrying counts", list)
	}
}

func TestHandleRenameTag_Duplicate_Conflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tag := createTagForRename(t, rt, cookie, "production")
	createTagForRename(t, rt, cookie, "staging")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPatch, "/api/v1/tags/"+tag.ID, `{"name":"staging"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}
