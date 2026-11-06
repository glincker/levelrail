package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestNormalizeVitalsRoute(t *testing.T) {
	cases := map[string]string{
		"/blog/123?x=1#top":                       "/blog/:id",
		"/u/0f8fad5b-d9cb-469f-a165-70867728950e": "/u/:id",
		"pricing": "/pricing",
	}
	for in, want := range cases {
		if got := normalizeVitalsRoute(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVitalsReportRatings(t *testing.T) {
	// LCP bucket 4 is <=2500 (good), bucket 7 is <=5000 (poor).
	overall, routes := buildVitalsReport([]store.VitalsBucketCount{
		{Route: "/", Metric: "LCP", Bucket: 4, Count: 8},
		{Route: "/", Metric: "LCP", Bucket: 7, Count: 2},
		{Route: "/slow", Metric: "LCP", Bucket: 7, Count: 5},
	})
	if got := overall["LCP"]; got.Count != 15 || got.Rating != "poor" {
		t.Fatalf("overall LCP = %+v", got)
	}
	if len(routes) != 2 || routes[0].Route != "/slow" && routes[0].Route != "/" {
		t.Fatalf("routes = %+v", routes)
	}
	for _, r := range routes {
		if r.Route == "/" && r.Metrics["LCP"].Rating != "good" {
			t.Fatalf("route / rating = %q", r.Metrics["LCP"].Rating)
		}
	}
}

func TestVitalsBeaconFlow(t *testing.T) {
	rt, db := newTestRouter(t)
	seedBoundTestToken(t, db, "tok_vit_dep", "plain-vit-dep", AbilityRead)
	const app = "vitalsapp"
	if err := rt.apps.SaveDesiredService(t.Context(), store.DesiredService{Name: app, Image: "nginx"}); err != nil {
		t.Fatal(err)
	}
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := post("/api/v1/vitals/"+app, `{"key":"x","m":[{"n":"LCP","v":1200}]}`); rec.Code != http.StatusNotFound {
		t.Fatalf("disabled beacon = %d", rec.Code)
	}
	cfg, err := rt.apps.(vitalsStore).SetVitalsEnabled(t.Context(), app, true)
	if err != nil {
		t.Fatal(err)
	}
	if rec := post("/api/v1/vitals/"+app, `{"key":"wrong","m":[{"n":"LCP","v":1200}]}`); rec.Code != http.StatusNotFound {
		t.Fatalf("wrong key = %d", rec.Code)
	}
	body := `{"key":"` + cfg.PublicKey + `","path":"/p/42?q=1","m":[{"n":"LCP","v":1200},{"n":"BOGUS","v":1}]}`
	if rec := post("/api/v1/vitals/"+app, body); rec.Code != http.StatusNoContent {
		t.Fatalf("beacon = %d %s", rec.Code, rec.Body.String())
	}
	rec := bearerGet(rt, "/api/v1/apps/"+app+"/vitals", "plain-vit-dep")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"/p/:id"`) || !strings.Contains(rec.Body.String(), `"rating":"good"`) {
		t.Fatalf("report = %d %s", rec.Code, rec.Body.String())
	}
}
