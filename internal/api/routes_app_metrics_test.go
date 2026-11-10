package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppScrapeTargetRoutes(t *testing.T) {
	f := newTeamFixture(t)
	dev, viewer := f.cookies[store.TeamRoleDeveloper], f.cookies[store.TeamRoleViewer]
	as := func(c *http.Cookie) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(c)
			r.Header.Set("Content-Type", "application/json")
		}
	}
	path := "/api/v1/apps/" + appA + "/scrape-target"

	for _, body := range []string{`{"port":0,"path":"/metrics"}`, `{"port":70000,"path":"/metrics"}`, `{"port":9100,"path":"//evil.example/m"}`, `{"port":9100,"path":"/m?x=1"}`} {
		if code, raw := teamDo(t, f.rt, http.MethodPut, path, body, as(dev)); code != http.StatusBadRequest {
			t.Fatalf("PUT %s = %d %s, want 400", body, code, raw)
		}
	}
	if code, raw := teamDo(t, f.rt, http.MethodPut, path, `{"port":9100,"path":"/metrics"}`, as(viewer)); code != http.StatusForbidden {
		t.Fatalf("viewer PUT = %d %s, want 403", code, raw)
	}
	if code, raw := teamDo(t, f.rt, http.MethodPut, path, `{"port":9100,"path":"/metrics"}`, as(dev)); code != http.StatusOK {
		t.Fatalf("PUT = %d %s", code, raw)
	}
	if code, raw := teamDo(t, f.rt, http.MethodGet, path, "", as(viewer)); code != http.StatusOK || !strings.Contains(raw, `"configured":true`) || !strings.Contains(raw, `"port":9100`) {
		t.Fatalf("viewer GET = %d %s", code, raw)
	}
	if code, raw := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appA+"/scraped-metrics", "", as(viewer)); code != http.StatusOK {
		t.Fatalf("scraped-metrics = %d %s", code, raw)
	}
	if code, raw := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appA+"/scraped-metrics?metric=bad-name", "", as(viewer)); code != http.StatusBadRequest {
		t.Fatalf("bad metric name = %d %s, want 400", code, raw)
	}
	for _, p := range []string{"/api/v1/apps/" + appB + "/scrape-target", "/api/v1/apps/" + appB + "/scraped-metrics"} {
		if code, raw := teamDo(t, f.rt, http.MethodGet, p, "", as(dev)); code != http.StatusNotFound {
			t.Fatalf("other team GET %s = %d %s, want 404", p, code, raw)
		}
	}
	if code, raw := teamDo(t, f.rt, http.MethodDelete, path, "", as(dev)); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d %s", code, raw)
	}
	if code, raw := teamDo(t, f.rt, http.MethodGet, path, "", as(dev)); code != http.StatusOK || !strings.Contains(raw, `"configured":false`) {
		t.Fatalf("GET after delete = %d %s", code, raw)
	}
}
