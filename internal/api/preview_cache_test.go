package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/preview"
	"github.com/GLINCKER/levelrail/internal/store"
)

func previewGet(t *testing.T, rt *Router, cookie *http.Cookie, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := authedRequest(t, cookie, http.MethodGet, target, "")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestPreviewImage_ETagAndNotModified(t *testing.T) {
	rt, db, fp := newPreviewTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	fp.addOK(t, "web", "dep_a")
	base := "/api/v1/apps/web/deployments/dep_a/preview"

	first := previewGet(t, rt, cookie, base+"?v=1", nil)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" || etag[0] != '"' {
		t.Fatalf("first = %d etag %q, want 200 with strong etag", first.Code, etag)
	}
	again := previewGet(t, rt, cookie, base+"?v=1", map[string]string{"If-None-Match": etag})
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Errorf("conditional = %d body %d, want 304 empty", again.Code, again.Body.Len())
	}
	stale := previewGet(t, rt, cookie, base+"?v=1", map[string]string{"If-None-Match": `"other"`})
	if stale.Code != http.StatusOK {
		t.Errorf("mismatched etag = %d, want 200", stale.Code)
	}
}

func TestPreviewImage_CacheControlByVersionParam(t *testing.T) {
	rt, db, fp := newPreviewTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	fp.addOK(t, "web", "dep_a")
	base := "/api/v1/apps/web/deployments/dep_a/preview"

	if got := previewGet(t, rt, cookie, base+"?v=1700000000", nil).Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Errorf("versioned cache-control = %q", got)
	}
	if got := previewGet(t, rt, cookie, base, nil).Header().Get("Cache-Control"); got != "private, max-age=86400, stale-while-revalidate=604800" {
		t.Errorf("unversioned cache-control = %q", got)
	}
}

func TestPreviewImage_RangeAndAuth(t *testing.T) {
	rt, db, fp := newPreviewTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	fp.addOK(t, "web", "dep_a")
	base := "/api/v1/apps/web/deployments/dep_a/preview"

	rec := previewGet(t, rt, cookie, base, map[string]string{"Range": "bytes=0-1"})
	if rec.Code != http.StatusPartialContent || rec.Body.Len() != 2 {
		t.Errorf("range = %d len %d, want 206 len 2", rec.Code, rec.Body.Len())
	}
	anon := httptest.NewRecorder()
	rt.Handler().ServeHTTP(anon, httptest.NewRequest(http.MethodGet, base+"?v=1", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Errorf("anonymous = %d, want 401", anon.Code)
	}
}

type latestPreview struct {
	*fakePreview
}

func (l latestPreview) LatestRenderable(_ context.Context, apps []string) (map[string]preview.Record, error) {
	out := map[string]preview.Record{}
	for _, r := range l.records {
		for _, a := range apps {
			if r.App == a {
				out[a] = r
			}
		}
	}
	return out, nil
}

func TestListPreviewCards_ImageAndCard(t *testing.T) {
	fp := newFakePreview(t)
	fp.addOK(t, "web", "dep_a")
	fp.records["dep_c"] = preview.Record{DeploymentID: "dep_c", App: "docs", Status: preview.StatusOK, Source: preview.SourceCard, Meta: `{"title":"Docs"}`}
	rt := &Router{preview: latestPreview{fp}}

	got := rt.listPreviewCards(context.Background(), []store.DesiredService{{Name: "web"}, {Name: "docs"}, {Name: "none"}})
	if got["web"].ImageURL == "" {
		t.Errorf("web image url empty: %+v", got["web"])
	}
	if got["docs"].Card == nil || got["docs"].Card.Title != "Docs" || got["docs"].ImageURL != "" {
		t.Errorf("docs card = %+v", got["docs"])
	}
	if _, ok := got["none"]; ok {
		t.Error("app without preview must be absent")
	}
	if (&Router{preview: fp}).listPreviewCards(context.Background(), []store.DesiredService{{Name: "web"}}) != nil {
		t.Error("service without the batch method must yield nil")
	}
}
