package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedTagApp(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app %q: %v", name, err)
	}
}

func TestHandleCreateTag_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/tags", `{"name":"production"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got tagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" || got.Name != "production" {
		t.Errorf("created resource = %+v, unexpected", got)
	}
}

func TestHandleCreateTag_EmptyName_BadRequest(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/tags", `{"name":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateTag_DuplicateName_Conflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/tags", `{"name":"production"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/tags", `{"name":"production"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleListTags(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for _, name := range []string{"staging", "production"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/tags", `{"name":"`+name+`"}`))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %q status = %d, want %d", name, rec.Code, http.StatusCreated)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/tags", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []tagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Name != "production" || got[1].Name != "staging" {
		t.Fatalf("list = %+v, want alphabetical [production, staging]", got)
	}
}

func TestHandleDeleteTag(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/tags", `{"name":"production"}`))
	var created tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/tags/"+created.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/tags/"+created.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAttachAppTag_CreatesAndAttaches(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTagApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tags", `{"name":"production"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var attached tagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &attached); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if attached.Name != "production" {
		t.Errorf("attached tag = %+v, want name production", attached)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("get app status = %d, want %d", rec.Code, http.StatusOK)
	}
	var app appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode app: %v", err)
	}
	if len(app.Tags) != 1 || app.Tags[0] != "production" {
		t.Errorf("app.Tags = %+v, want [production]", app.Tags)
	}
}

func TestHandleAttachAppTag_ReusesExistingTagByName(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTagApp(t, db, "web")
	seedTagApp(t, db, "worker")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tags", `{"name":"production"}`))
	var first tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &first)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/worker/tags", `{"name":"production"}`))
	var second tagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("second attach ID = %q, want reused %q", second.ID, first.ID)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/tags", ""))
	var all []tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &all)
	if len(all) != 1 {
		t.Fatalf("GET /api/v1/tags = %+v, want exactly one tag (reused, not duplicated)", all)
	}
}

func TestHandleAttachAppTag_UnknownApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nonexistent/tags", `{"name":"production"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleDetachAppTag(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTagApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tags", `{"name":"production"}`))
	var tag tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/tags/"+tag.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/tags", ""))
	var remaining []tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &remaining)
	if len(remaining) != 0 {
		t.Fatalf("GET /api/v1/apps/web/tags after detach = %+v, want empty", remaining)
	}
}

func TestHandleDetachAppTag_NotAttached_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTagApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/tags/tag_missing", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListAppsByTag(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTagApp(t, db, "web")
	seedTagApp(t, db, "worker")
	seedTagApp(t, db, "batch")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tags", `{"name":"production"}`))
	var tag tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/worker/tags", `{"name":"production"}`))

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/tags/"+tag.ID+"/apps", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var apps []tagAppResource
	if err := json.Unmarshal(rec.Body.Bytes(), &apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(apps) != 2 || apps[0].Name != "web" || apps[1].Name != "worker" {
		t.Fatalf("apps for tag = %+v, want [web, worker], not batch", apps)
	}
}

func TestHandleListAppsByTag_UnknownTag_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/tags/tag_missing/apps", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListApps_IncludesTags(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTagApp(t, db, "web")
	seedTagApp(t, db, "worker")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tags", `{"name":"production"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var apps []appListResource
	if err := json.Unmarshal(rec.Body.Bytes(), &apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byName := make(map[string]appListResource, len(apps))
	for _, a := range apps {
		byName[a.Name] = a
	}
	if len(byName["web"].Tags) != 1 || byName["web"].Tags[0] != "production" {
		t.Errorf("web.Tags = %+v, want [production]", byName["web"].Tags)
	}
	if len(byName["worker"].Tags) != 0 {
		t.Errorf("worker.Tags = %+v, want empty", byName["worker"].Tags)
	}
}
