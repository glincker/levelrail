package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestNormalizeVitalsRoute(t *testing.T) {
	cases := map[string]string{
		"/blog/123?x=1#top":                       "/blog/:id",
		"/u/0f8fad5b-d9cb-469f-a165-70867728950e": "/u/:id",
		"pricing":                               "/pricing",
		"/blog/my-post":                         "/blog/my-post",
		"/s/AbC123":                             "/s/:id",
		"/s/dGhpcy1pcy1iYXNlNjR1cmw":            "/s/:id",
		"/u/alice@example.com":                  "/u/:id",
		"/reset/eyJhbGciOi.eyJzdWIiOi.SflKxwRJ": "/reset/:id",
		"/x/abcdefghijklmnopqrstuvwxyz":         "/x/:id",
		"/caf\u00e9/menu":                       "/:id/menu",
		"/a/b/c/d/e/f/g/h":                      "/a/b/c/d/e/f",
		"/":                                     "/",
		"/v2/ok":                                "/:id/ok",
	}
	for in, want := range cases {
		if got := normalizeVitalsRoute(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVitalsTruncateOnRuneBoundary(t *testing.T) {
	got := truncateVitalsRoute("ab\u00e9cd", 3)
	if got != "ab" || !utf8.ValidString(got) {
		t.Fatalf("truncate = %q", got)
	}
}

func TestVitalsOverflowPercentile(t *testing.T) {
	// LCP has 10 bounds, so bucket 10 is above the 10 s bound.
	st := vitalsStatOf("LCP", map[int]int64{10: 9, 0: 1})
	if !st.Overflow || !st.P95Overflow || st.P75 != 10000 || st.Rating != "poor" {
		t.Fatalf("stat = %+v", st)
	}
	if st := vitalsStatOf("LCP", map[int]int64{1: 4}); st.Overflow {
		t.Fatalf("unexpected overflow %+v", st)
	}
}

func TestVitalsRouteGateFoldsExtras(t *testing.T) {
	rt, _ := newTestRouter(t)
	db := rt.apps.(vitalsStore)
	g := newVitalsRouteGate()
	now := time.Now()
	for _, r := range []string{"/a", "/b", "/a"} {
		if got := g.admit(t.Context(), db, "app", r, 2, now); got != r {
			t.Fatalf("admit(%q) = %q", r, got)
		}
	}
	if got := g.admit(t.Context(), db, "app", "/c", 2, now); got != "/other" {
		t.Fatalf("over cap = %q", got)
	}
}

func TestVitalsBufferFlushAggregates(t *testing.T) {
	rt, _ := newTestRouter(t)
	db := rt.apps.(vitalsStore)
	v := newVitalsBuffer(2)
	hour := time.Now().UTC().Truncate(time.Hour)
	k := vitalsKey{app: "a", route: "/", metric: "LCP", device: "desktop", hour: hour, bucket: 1}
	v.add(k, 1)
	v.add(k, 1)
	v.add(vitalsKey{app: "a", route: "/x", metric: "LCP", device: "desktop", hour: hour}, 1)
	v.add(vitalsKey{app: "a", route: "/dropped", metric: "LCP", device: "desktop", hour: hour}, 1)
	rtm := &vitalsRuntime{buf: v}
	if err := rtm.flush(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryVitals(t.Context(), "a", hour.Add(-time.Hour), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	for _, r := range rows {
		if r.Route == "/" && r.Count != 2 {
			t.Fatalf("aggregate = %+v", r)
		}
	}
}

func TestVitalsBatchPrune(t *testing.T) {
	rt, _ := newTestRouter(t)
	db := rt.apps.(vitalsStore)
	old := time.Now().Add(-72 * time.Hour)
	var rows []store.VitalsCount
	for i := range 7 {
		rows = append(rows, store.VitalsCount{App: "a", Route: "/", Metric: "LCP", Device: "desktop", Hour: old.Add(-time.Duration(i) * time.Hour), Bucket: 1, Count: 1})
	}
	if err := db.AddVitalsCounts(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	n, err := pruneVitals(t.Context(), db, time.Now().Add(-24*time.Hour), 3, time.Millisecond)
	if err != nil || n != 7 {
		t.Fatalf("pruned %d, %v", n, err)
	}
}

type errDomains struct{ DomainStore }

func (errDomains) ListServiceDomains(context.Context) ([]store.ServiceDomain, error) {
	return nil, errors.New("boom")
}

func TestVitalsOriginStoreErrorDenies(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.domains = errDomains{rt.domains}
	if rt.vitalsOriginAllowed(t.Context(), "app", "https://example.com") {
		t.Fatal("store error must deny")
	}
	if !rt.vitalsOriginAllowed(t.Context(), "app", "http://localhost:3000") {
		t.Fatal("localhost always allowed")
	}
}

func TestVitalsRateLimits(t *testing.T) {
	rt, _ := newTestRouter(t)
	const app = "ratelimited"
	if err := rt.apps.SaveDesiredService(t.Context(), store.DesiredService{Name: app, Image: "nginx"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := rt.apps.(vitalsStore).SetVitalsEnabled(t.Context(), app, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.runtimeSettings.Set(t.Context(), KeyVitalsRatePerMin, json.RawMessage(`3`), "test"); err != nil {
		t.Fatal(err)
	}
	post := func(key, addr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vitals/"+app, strings.NewReader(`{"key":"`+key+`","m":[{"n":"LCP","v":10}]}`))
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for range 20 {
		if code := post("wrong", "198.51.100.1:1"); code != http.StatusNotFound {
			t.Fatalf("junk = %d", code)
		}
	}
	for i := range 3 {
		if code := post(cfg.PublicKey, "198.51.100.2:1"); code != http.StatusNoContent {
			t.Fatalf("beacon %d = %d", i, code)
		}
	}
	if code := post(cfg.PublicKey, "198.51.100.2:1"); code != http.StatusTooManyRequests {
		t.Fatalf("over app limit = %d", code)
	}
	if err := rt.apps.SaveDesiredService(t.Context(), store.DesiredService{Name: "ipapp", Image: "nginx"}); err != nil {
		t.Fatal(err)
	}
	cfg2, err := rt.apps.(vitalsStore).SetVitalsEnabled(t.Context(), "ipapp", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.runtimeSettings.Set(t.Context(), KeyVitalsRatePerMin, json.RawMessage(`1000`), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.runtimeSettings.Set(t.Context(), KeyVitalsRatePerIP, json.RawMessage(`2`), "test"); err != nil {
		t.Fatal(err)
	}
	post = func(key, addr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vitals/ipapp", strings.NewReader(`{"key":"`+key+`","m":[{"n":"LCP","v":10}]}`))
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	cfg = cfg2
	codes := []int{post(cfg.PublicKey, "198.51.100.9:1"), post(cfg.PublicKey, "198.51.100.9:1"), post(cfg.PublicKey, "198.51.100.9:1")}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("per ip codes = %v", codes)
	}
	if post(cfg.PublicKey, "198.51.100.10:1") != http.StatusNoContent {
		t.Fatal("other ip must still pass")
	}
}

func TestVitalsLimiterEvictionKeepsActiveWindow(t *testing.T) {
	l := newAPIRateLimiter(2)
	l.maxKeys = 50
	l.allow("victim")
	l.allow("victim")
	for i := range 40 {
		l.allow("junk" + strconv.Itoa(i))
	}
	if ok, _ := l.allow("victim"); ok {
		t.Fatal("junk keys reset the victim window")
	}
}

func TestVitalsReportRatings(t *testing.T) {
	// LCP bucket 4 is <=2500 (good), bucket 7 is <=5000 (poor).
	rows := []store.VitalsBucketCount{
		{Route: "/", Metric: "LCP", Bucket: 4, Count: 8},
		{Route: "/", Metric: "LCP", Bucket: 7, Count: 2},
		{Route: "/slow", Metric: "LCP", Bucket: 7, Count: 5},
	}
	overall, routes := buildVitalsReport(rows, rows)
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
	if err := rt.vitalsRuntime().flush(t.Context(), rt.apps.(vitalsStore)); err != nil {
		t.Fatal(err)
	}
	rec := bearerGet(rt, "/api/v1/apps/"+app+"/vitals", "plain-vit-dep")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"/p/:id"`) || !strings.Contains(rec.Body.String(), `"rating":"good"`) {
		t.Fatalf("report = %d %s", rec.Code, rec.Body.String())
	}
}

func TestVitalsDropAppClearsBufferedCounters(t *testing.T) {
	rt, _ := newTestRouter(t)
	db := rt.apps.(vitalsStore)
	v := rt.vitalsRuntime()
	v.buf.add(vitalsKey{app: "gone", route: "/", metric: "LCP", device: "desktop", hour: time.Now().UTC().Truncate(time.Hour)}, 3)
	if err := v.dropApp(t.Context(), db, "gone"); err != nil {
		t.Fatal(err)
	}
	if err := v.flush(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryVitalsOverall(t.Context(), "gone", time.Now().Add(-2*time.Hour))
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows after drop = %v, %v", rows, err)
	}
}
