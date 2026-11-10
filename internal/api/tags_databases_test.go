package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseTags_TagListByTagAndCleanup(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, n := range []string{"main", "cache", "other"} {
		if err := db.SaveDesiredDatabase(t.Context(), store.DesiredDatabase{Name: n, Engine: "postgres", Version: "16"}); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := call(http.MethodPost, "/api/v1/databases/main/tags", `{"name":"Team:Billing"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("attach = %d, body = %s", rec.Code, rec.Body.String())
	}
	var tag tagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)
	if tag.Name != "team:billing" {
		t.Fatalf("tag name = %q, want normalized team:billing", tag.Name)
	}
	if rec := call(http.MethodPost, "/api/v1/databases/cache/tags", `{"name":"team:billing"}`); rec.Code != http.StatusCreated {
		t.Fatalf("attach cache = %d", rec.Code)
	}
	if rec := call(http.MethodPost, "/api/v1/databases/main/tags", `{"name":"bad tag!"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid tag = %d, want 400", rec.Code)
	}
	if rec := call(http.MethodPost, "/api/v1/databases/missing/tags", `{"name":"x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing database = %d, want 404", rec.Code)
	}

	rec = call(http.MethodGet, "/api/v1/tags/"+tag.ID+"/databases", "")
	var byTag []tagDatabaseResource
	_ = json.Unmarshal(rec.Body.Bytes(), &byTag)
	if rec.Code != http.StatusOK || len(byTag) != 2 || byTag[0].Name != "cache" || byTag[1].Name != "main" {
		t.Fatalf("by tag = %d %+v, want cache and main", rec.Code, byTag)
	}

	rec = call(http.MethodGet, "/api/v1/databases?tag=team:billing", "")
	var listed []databaseListResource
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if rec.Code != http.StatusOK || len(listed) != 2 || len(listed[0].Tags) != 1 {
		t.Fatalf("list ?tag = %d %+v, want two tagged rows", rec.Code, listed)
	}

	// Deleting the database must drop its join row so a reused name starts untagged.
	if err := db.DeleteDesiredDatabase(t.Context(), "cache"); err != nil {
		t.Fatalf("delete database: %v", err)
	}
	names, err := db.ListResourceNamesByTag(t.Context(), tag.ID, store.ResourceKindDatabase)
	if err != nil || len(names) != 1 || names[0] != "main" {
		t.Fatalf("after database delete = %v %v, want [main]", names, err)
	}

	if rec := call(http.MethodDelete, "/api/v1/databases/main/tags/"+tag.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("detach = %d", rec.Code)
	}
	if rec := call(http.MethodDelete, "/api/v1/databases/main/tags/"+tag.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second detach = %d, want 404", rec.Code)
	}

	// Deleting the tag cascades its remaining join rows.
	if rec := call(http.MethodPost, "/api/v1/databases/other/tags", `{"name":"team:billing"}`); rec.Code != http.StatusCreated {
		t.Fatalf("attach other = %d", rec.Code)
	}
	if rec := call(http.MethodDelete, "/api/v1/tags/"+tag.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete tag = %d", rec.Code)
	}
	tags, err := db.ListTagsForResource(t.Context(), store.ResourceRef{Kind: store.ResourceKindDatabase, Name: "other"})
	if err != nil || len(tags) != 0 {
		t.Fatalf("after tag delete = %v %v, want none", tags, err)
	}
}
