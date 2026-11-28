package ingress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/trafficclass"
)

const chromeAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

func serveOnce(t *testing.T, key string, r *http.Request, next caddyhttp.HandlerFunc) AccessRecord {
	t.Helper()
	log := DefaultAccessLog()
	got := make(chan AccessRecord, 1)
	unsub := log.Subscribe(func(k string, rec AccessRecord) {
		if k == key {
			got <- rec
		}
	})
	defer unsub()
	_ = requestStatsModule{Key: key}.ServeHTTP(httptest.NewRecorder(), r, next)
	select {
	case rec := <-got:
		return rec
	case <-time.After(time.Second):
		t.Fatal("no access record")
		return AccessRecord{}
	}
}

func TestRequestStatsRecordsClass(t *testing.T) {
	html := func(w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html></html>"))
		return nil
	}
	jsonNext := func(w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
		return nil
	}
	cases := []struct {
		name string
		path string
		h    map[string]string
		next caddyhttp.HandlerFunc
		want trafficclass.Class
	}{
		{"navigation", "/pricing", map[string]string{"Sec-Fetch-Dest": "document"}, html, trafficclass.Page},
		{"dashboard poll", "/dashboard/stats", map[string]string{"Sec-Fetch-Dest": "empty"}, jsonNext, trafficclass.API},
		{"api path", "/api/v1/apps", map[string]string{"Sec-Fetch-Dest": "document"}, jsonNext, trafficclass.API},
		{"old browser html", "/about", map[string]string{"Accept": "text/html,*/*"}, html, trafficclass.Page},
		{"bot", "/", map[string]string{"User-Agent": "Googlebot/2.1"}, html, trafficclass.Bot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.Header.Set("User-Agent", chromeAgent)
			for k, v := range tc.h {
				r.Header.Set(k, v)
			}
			rec := serveOnce(t, "class-"+tc.name+".test", r, tc.next)
			if rec.Class != tc.want {
				t.Fatalf("class = %s, want %s", rec.Class, tc.want)
			}
		})
	}
}

func TestPerKeyTrafficRulesOverrideDefaults(t *testing.T) {
	rules, err := trafficclass.NewRules([]trafficclass.Rule{{Pattern: "/api/*", Class: trafficclass.Page}})
	if err != nil {
		t.Fatal(err)
	}
	SetTrafficRules(map[string]*trafficclass.Rules{"rules.test": rules})
	t.Cleanup(func() { SetTrafficRules(nil) })
	r := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	r.Header.Set("User-Agent", chromeAgent)
	rec := serveOnce(t, "rules.test", r, func(http.ResponseWriter, *http.Request) error { return nil })
	if rec.Class != trafficclass.Page {
		t.Fatalf("class = %s, want page from the per-app rule", rec.Class)
	}
}

func TestSummarizeSplitsClassesAndRanksPagesOnly(t *testing.T) {
	l := NewAccessLog(100)
	now := time.Now()
	add := func(class trafficclass.Class, method, path string, status int) {
		l.Record("k", AccessRecord{Time: now.Add(-time.Minute), Host: "a.test", Method: method, Path: path, Status: status,
			Class: class, Client: netip.MustParseAddr("10.0.0.1")})
	}
	for i := 0; i < 6; i++ {
		add(trafficclass.API, "GET", "/api/v1/stats", 200)
	}
	add(trafficclass.Asset, "GET", "/assets/app.js", 200)
	add(trafficclass.Bot, "GET", "/", 200)
	add(trafficclass.Prefetch, "GET", "/pricing", 200)
	add(trafficclass.Page, "GET", "/pricing", 200)
	add(trafficclass.Page, "GET", "/pricing/", 200)
	add(trafficclass.Page, "GET", "/blog/post-1234567", 200)
	add(trafficclass.Page, "GET", "/missing", 404)
	add(trafficclass.Page, "GET", "/old", 301)
	add(trafficclass.Page, "POST", "/form", 200)
	add(trafficclass.Ignored, "GET", "/healthz", 200)
	sum := l.Summarize([]string{"k"}, VisitorSummaryOptions{Since: now.Add(-time.Hour), Until: now})
	want := ClassCounts{Page: 6, API: 6, Asset: 1, Bot: 1, Prefetch: 1}
	if sum.Classes != want {
		t.Fatalf("classes = %+v, want %+v", sum.Classes, want)
	}
	if sum.TopPaths[0].Label != "/api/v1/stats" {
		t.Fatalf("top_paths must stay unfiltered for older clients: %+v", sum.TopPaths)
	}
	var labels []string
	for _, p := range sum.TopPages {
		labels = append(labels, p.Label)
		if strings.HasPrefix(p.Label, "/api") || strings.HasPrefix(p.Label, "/assets") {
			t.Errorf("non-page path in top_pages: %q", p.Label)
		}
	}
	if len(sum.TopPages) != 2 || sum.TopPages[0] != (CountedItem{Label: "/pricing", Count: 2}) || sum.TopPages[1].Label != "/blog/:id" {
		t.Fatalf("top_pages = %v (labels %v)", sum.TopPages, labels)
	}
}

type fakeRollupStore struct {
	mu   sync.Mutex
	rows []store.VisitorRollup
}

func (f *fakeRollupStore) UpsertVisitorRollups(_ context.Context, rows []store.VisitorRollup) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, rows...)
	return nil
}

func (f *fakeRollupStore) ListVisitorRollups(context.Context, string, string, time.Time, time.Time) ([]store.VisitorRollup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.VisitorRollup(nil), f.rows...), nil
}

func (f *fakeRollupStore) PruneVisitorRollups(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func TestRollupCarriesClassesAndPageLabels(t *testing.T) {
	t.Setenv("APP_TRAFFIC_PAGE_LABELS", "2")
	st := &fakeRollupStore{}
	l := NewAccessLog(10)
	l.SetVisitorHistory(st, func(string) string { return "web" }, func() time.Duration { return time.Hour }, nil)
	at := time.Now().Add(-90 * time.Minute)
	rec := func(class trafficclass.Class, path string) {
		l.Record("k", AccessRecord{Time: at, Host: "a.test", Method: "GET", Path: path, Status: 200, Class: class, Client: netip.MustParseAddr("10.0.0.9")})
	}
	rec(trafficclass.API, "/api/x")
	rec(trafficclass.API, "/api/x")
	rec(trafficclass.Page, "/a")
	rec(trafficclass.Page, "/b")
	rec(trafficclass.Page, "/c")
	rec(trafficclass.Page, "/d")
	if err := l.FlushVisitorHistory(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(st.rows) != 1 {
		t.Fatalf("rows = %d", len(st.rows))
	}
	row := st.rows[0]
	if row.Classes[trafficclass.API] != 2 || row.Classes[trafficclass.Page] != 4 {
		t.Fatalf("classes = %v", row.Classes)
	}
	got := map[string]int{}
	for _, c := range row.TopPages {
		got[c.Label] = c.Count
	}
	if len(got) != 3 || got["/a"] != 1 || got["/b"] != 1 || got[otherLabel] != 2 {
		t.Fatalf("top_pages = %v, want /a, /b and (other)=2 past the label cap", got)
	}
	if row.Requests != 6 {
		t.Fatalf("requests = %d", row.Requests)
	}
}

func TestMergeRollupsAddsClassesAndPages(t *testing.T) {
	now := time.Now()
	live := VisitorSummary{Status: map[string]int{"2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0}, Series: make([]VisitorBucket, visitorSeriesBuckets)}
	live.Classes = ClassCounts{Page: 1}
	live.TopPages = []CountedItem{{Label: "/a", Count: 1}}
	rows := []store.VisitorRollup{{
		App: "web", Domain: "a.test", HourStart: now.Add(-3 * time.Hour).Truncate(time.Hour), Epoch: 1, Requests: 10,
		Classes: [6]int{0, 4, 5, 1, 0, 0}, TopPages: []store.RollupCount{{Label: "/a", Count: 3}, {Label: "/b", Count: 1}},
	}}
	got := mergeRollups(live, rows, 2, now, now.Add(-24*time.Hour), now, 10)
	if got.Classes != (ClassCounts{Page: 5, API: 5, Asset: 1}) {
		t.Fatalf("classes = %+v", got.Classes)
	}
	if len(got.TopPages) != 2 || got.TopPages[0] != (CountedItem{Label: "/a", Count: 4}) {
		t.Fatalf("top_pages = %+v", got.TopPages)
	}
}
