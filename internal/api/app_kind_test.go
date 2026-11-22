package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestClassifyApp(t *testing.T) {
	t.Parallel()
	dom := []string{"a.example.com"}
	cases := []struct {
		name       string
		in         appKindInput
		wantKind   string
		wantSource string
	}{
		{"static build", appKindInput{BuildType: "static", Domains: dom}, appKindStatic, kindSourceSpec},
		{"cron only", appKindInput{HasJobs: true}, appKindCron, kindSourceSpec},
		{"jobs but serves", appKindInput{HasJobs: true, Port: 80, Domains: dom}, appKindWeb, kindSourceHeuristic},
		{"postgres", appKindInput{Image: "postgres:16", Port: 5432}, appKindDatabase, kindSourceHeuristic},
		{"registry redis digest", appKindInput{Image: "docker.io/library/redis@sha256:abc", Port: 6379}, appKindDatabase, kindSourceHeuristic},
		{"db image with domain is not db", appKindInput{Image: "postgres:16", Port: 80, Domains: dom}, appKindWeb, kindSourceHeuristic},
		{"worker", appKindInput{Image: "acme/queue:1"}, appKindWorker, kindSourceSpec},
		{"html probe", appKindInput{Port: 80, Domains: dom, ContentType: "text/html; charset=utf-8"}, appKindWeb, kindSourceProbe},
		{"json probe", appKindInput{Port: 80, Domains: dom, ContentType: "application/json"}, appKindAPI, kindSourceProbe},
		{"api path", appKindInput{Port: 80, Domains: dom, HealthURL: "/api/health"}, appKindAPI, kindSourceHeuristic},
		{"v1 path", appKindInput{Port: 80, HealthURL: "/v1"}, appKindAPI, kindSourceHeuristic},
		{"health no domain", appKindInput{Port: 8080, HealthURL: "/healthz"}, appKindAPI, kindSourceHeuristic},
		{"health with domain stays web", appKindInput{Port: 80, Domains: dom, HealthURL: "/healthz"}, appKindWeb, kindSourceHeuristic},
		{"plain web", appKindInput{Port: 3000, Domains: dom, HealthURL: "/"}, appKindWeb, kindSourceHeuristic},
		{"apiary is not api", appKindInput{Port: 80, Domains: dom, HealthURL: "/apiary"}, appKindWeb, kindSourceHeuristic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			k, s := classifyApp(tc.in)
			if k != tc.wantKind || s != tc.wantSource {
				t.Errorf("got %s/%s, want %s/%s", k, s, tc.wantKind, tc.wantSource)
			}
		})
	}
}

func TestFoldJobsNextAndLast(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	ran := now.Add(-time.Hour)
	got := foldJobs([]store.ScheduledTask{
		{ServiceName: "a", Schedule: "0 12 * * *", LastRunAt: &ran, LastRunStatus: "success"},
		{ServiceName: "a", Schedule: "0 11 * * *"},
	}, now)
	js := got["a"]
	if js == nil || js.count != 2 || js.next == nil || !js.next.Equal(time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)) || js.lastRun != "success" {
		t.Fatalf("summary = %+v", js)
	}
}

func listApps(t *testing.T, rt *Router, db *store.DB) []map[string]any {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestListAppsCarriesKindAndSummary(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	for _, s := range []store.DesiredService{
		{Name: "site", Image: "acme/site:1", Port: 3000, Domains: []string{"site.example.com"}},
		{Name: "crunch", Image: "acme/crunch:1"},
		{Name: "pg", Image: "postgres:16", Port: 5432},
	} {
		if err := db.SaveDesiredService(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	kinds := map[string]string{}
	for _, a := range listApps(t, rt, db) {
		kinds[a["name"].(string)], _ = a["app_kind"].(string)
		if a["kind_source"] == "" || a["summary"] == nil {
			t.Errorf("%v missing kind_source or summary", a["name"])
		}
	}
	want := map[string]string{"site": "web", "crunch": "worker", "pg": "database"}
	for n, k := range want {
		if kinds[n] != k {
			t.Errorf("%s kind = %q, want %q", n, kinds[n], k)
		}
	}
}

// Sanity bound for the list path: classification is pure, so 200 apps must stay far inside the request budget.
func BenchmarkClassifyApp200(b *testing.B) {
	in := make([]appKindInput, 200)
	for i := range in {
		in[i] = appKindInput{Image: fmt.Sprintf("acme/app%d:1", i), Port: 80, Domains: []string{"x.example.com"}, HealthURL: "/healthz"}
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, x := range in {
			classifyApp(x)
		}
	}
}
