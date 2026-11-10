package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newAccessSearchFixture(t *testing.T) (*Router, string, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	stats := ingress.NewRequestStats()
	stats.SetHostOwners(map[string]string{"alpha.example.com": "alpha-app", "beta.example.com": "beta-app", "edge.example.com": ""})
	log := ingress.NewAccessLog(100)
	now := time.Now()
	for i, host := range []string{"alpha.example.com", "beta.example.com", "edge.example.com"} {
		for j := range 3 {
			log.Record(host, ingress.AccessRecord{
				Time: now.Add(-time.Duration(i*10+j) * time.Second), Host: host, Method: "GET",
				Path: "/api/item" + string(rune('a'+j)), Status: 200 + j*150, RequestID: host + "-" + string(rune('a'+j)),
			})
		}
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithAccessSource(EdgeAccess{Stats: stats, Log: log}))
	ctx := context.Background()
	for _, app := range []string{"alpha-app", "beta-app"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: "img:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	tok := seedMatrixToken(t, db, "scoped-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-beta-app", "Deny", "*", "app:beta-app", store.PrincipalTypeToken, "scoped-tok")
	return rt, tok, loginTestSession(t, rt, db)
}

func decodeAccessSearch(t *testing.T, w *httptest.ResponseRecorder) accessSearchResponse {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var out accessSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAccessSearchTenantSeesOnlyReadableHosts(t *testing.T) {
	rt, tok, cookie := newAccessSearchFixture(t)
	get := func(path string, scoped bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		if scoped {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			bearer(tok)(req)
			rt.Handler().ServeHTTP(w, req)
		} else {
			rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodGet, path, ""))
		}
		return w
	}
	body := get("/api/v1/logs/access", true).Body.String()
	if !strings.Contains(body, "alpha.example.com") || strings.Contains(body, "beta.example.com") || strings.Contains(body, "edge.example.com") {
		t.Fatalf("scoped search leaked or missed rows: %s", body)
	}
	for _, p := range []string{"/api/v1/logs/access?app=beta-app", "/api/v1/logs/access?app=nope-app"} {
		if got := decodeAccessSearch(t, get(p, true)); len(got.Records) != 0 {
			t.Fatalf("%s: hidden or unknown app returned rows", p)
		}
	}
	if w := get("/api/v1/apps/nope-app/access/search", true); w.Code != http.StatusNotFound {
		t.Fatalf("unknown app status %d, want 404", w.Code)
	}
	if w := get("/api/v1/apps/beta-app/access/search", true); w.Code != http.StatusForbidden {
		t.Fatalf("denied app status %d, want the house 403 for an explicit Deny", w.Code)
	}
	all := decodeAccessSearch(t, get("/api/v1/logs/access", false))
	if len(all.Records) != 9 {
		t.Fatalf("admin rows = %d, want 9", len(all.Records))
	}
	own := decodeAccessSearch(t, get("/api/v1/apps/alpha-app/access/search?status_min=500&path=/api/", true))
	if len(own.Records) != 1 || own.Records[0].Status < 500 || own.Records[0].App != "alpha-app" {
		t.Fatalf("filtered app rows = %+v", own.Records)
	}
}

func TestAccessSearchCapsAndCursor(t *testing.T) {
	rt, _, cookie := newAccessSearchFixture(t)
	get := func(q url.Values) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodGet, "/api/v1/logs/access?"+q.Encode(), ""))
		return w
	}
	t.Setenv("APP_ACCESS_SEARCH_MAX_WINDOW", "1h")
	t.Setenv("APP_ACCESS_SEARCH_MAX_TERM", "8")
	t.Setenv("APP_ACCESS_SEARCH_MAX_ROWS", "4")
	for _, bad := range []url.Values{
		{"since": {"2h"}},
		{"q": {strings.Repeat("x", 9)}},
		{"status": {"9xx"}},
		{"cursor": {"!!"}},
		{"method": {"G T"}},
	} {
		if w := get(bad); w.Code != http.StatusBadRequest {
			t.Fatalf("%v: status %d, want 400", bad, w.Code)
		}
	}
	seen := map[string]bool{}
	q := url.Values{"limit": {"50"}}
	for range 4 {
		page := decodeAccessSearch(t, get(q))
		if len(page.Records) > 4 || page.MaxRows != 4 {
			t.Fatalf("page of %d rows, max_rows %d", len(page.Records), page.MaxRows)
		}
		for _, r := range page.Records {
			if seen[r.RequestID] {
				t.Fatalf("row %s returned twice", r.RequestID)
			}
			seen[r.RequestID] = true
		}
		if page.NextCursor == "" {
			break
		}
		q.Set("cursor", page.NextCursor)
	}
	if len(seen) != 9 {
		t.Fatalf("paged %d rows, want 9", len(seen))
	}
}

func TestAccessSearchTextIsLiteral(t *testing.T) {
	log := ingress.NewAccessLog(10)
	now := time.Now()
	for i, p := range []string{"/a%b", "/axb", "/a_b"} {
		log.Record("k", ingress.AccessRecord{Time: now.Add(-time.Duration(i) * time.Second), Path: p, RequestID: p})
	}
	for text, want := range map[string]int{"%": 1, "_": 1, "a%b": 1, "A": 3} {
		res := log.Search([]string{"k"}, ingress.AccessSearch{From: now.Add(-time.Minute), Text: text, Limit: 10}, nil)
		if len(res.Records) != want {
			t.Fatalf("text %q matched %d, want %d", text, len(res.Records), want)
		}
	}
	res := log.Search([]string{"k"}, ingress.AccessSearch{From: now.Add(-time.Minute), Limit: 10, MaxScan: 2}, nil)
	if !res.Truncated || len(res.Records) != 2 {
		t.Fatalf("scan cap: truncated %v rows %d", res.Truncated, len(res.Records))
	}
}

func TestAccessSearchRecreatedAppSeesNoOldRows(t *testing.T) {
	stats := ingress.NewRequestStats()
	stats.SetHostOwners(map[string]string{"a.example.com": "alpha-app"})
	log := ingress.NewAccessLog(10)
	log.Record("a.example.com", ingress.AccessRecord{Time: time.Now(), Host: "a.example.com", Path: "/secret", RequestID: "old"})
	edge := EdgeAccess{Stats: stats, Log: log}
	// Delete and recreate inside one reconcile pass: the owner map never changes, the delete purge must.
	edge.PurgeApp("alpha-app")
	if got := edge.AppAccess("alpha-app", ingress.AccessFilter{}); len(got) != 0 {
		t.Fatalf("recreated app sees %d old rows", len(got))
	}
	log.Record("b.example.com", ingress.AccessRecord{Time: time.Now(), Host: "b.example.com", Path: "/prev", RequestID: "prev"})
	stats.SetHostOwners(map[string]string{"a.example.com": "alpha-app", "b.example.com": "alpha-app"})
	res := edge.SearchAccess(edge.AppKeys("alpha-app"), ingress.AccessSearch{From: time.Now().Add(-time.Hour), Limit: 10})
	if len(res.Records) != 0 {
		t.Fatalf("newly attached host leaked rows from before the attach: %+v", res.Records)
	}
}

func TestAccessSearchBusyAnswers429(t *testing.T) {
	rt, _, cookie := newAccessSearchFixture(t)
	t.Setenv("APP_ACCESS_SEARCH_MAX_CONCURRENT", "1")
	accessSearchInflight.Add(1)
	defer accessSearchInflight.Add(-1)
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodGet, "/api/v1/logs/access", ""))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", w.Code)
	}
}
