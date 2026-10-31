package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/search"
	"github.com/GLINCKER/levelrail/internal/store"
)

type searchResp struct {
	Hits []struct {
		Ref    string `json:"ref"`
		Kind   string `json:"kind"`
		Action *struct {
			ID      string `json:"id"`
			Ability string `json:"ability"`
			Confirm bool   `json:"confirm"`
		} `json:"action"`
	} `json:"hits"`
	NextCursor string `json:"next_cursor"`
}

func (s searchResp) refs() []string {
	var out []string
	for _, h := range s.Hits {
		out = append(out, h.Ref)
	}
	sort.Strings(out)
	return out
}

func doSearch(t *testing.T, rt *Router, tok, q string, extra ...string) searchResp {
	t.Helper()
	target := "/api/v1/search?q=" + url.QueryEscape(q)
	for _, e := range extra {
		target += "&" + e
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body.String())
	}
	var out searchResp
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func seedSearchApps(t *testing.T, db *store.DB, names ...string) {
	t.Helper()
	for _, n := range names {
		seedApp(t, db, n)
	}
}

func TestSearch_RestartAPIReturnsOnlyDeployableApps(t *testing.T) {
	rt, db := newTestRouter(t)
	seedSearchApps(t, db, "api", "api-gateway", "secret-api", "web")

	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	deployer := seedMatrixToken(t, db, "deployer", []string{AbilityRead, AbilityDeploy})
	reader := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-secret", "Deny", "*", "app:secret-api", store.PrincipalTypeToken, "deployer")

	got := doSearch(t, rt, root, "restart api", "kind=action")
	want := []string{"action:app.restart:api", "action:app.restart:api-gateway", "action:app.restart:secret-api"}
	if strings.Join(got.refs(), ",") != strings.Join(want, ",") {
		t.Fatalf("root restart api = %v, want %v", got.refs(), want)
	}
	if a := got.Hits[0].Action; a == nil || a.ID != "app.restart" || a.Ability != AbilityDeploy || !a.Confirm {
		t.Fatalf("action info = %+v", a)
	}

	// Template docs are global and match the "api" token; this test is about app actions.
	got = doSearch(t, rt, deployer, "restart api", "kind=action")
	want = []string{"action:app.restart:api", "action:app.restart:api-gateway"}
	if strings.Join(got.refs(), ",") != strings.Join(want, ",") {
		t.Fatalf("deployer with a Deny on secret-api = %v, want %v", got.refs(), want)
	}

	if got = doSearch(t, rt, reader, "restart api", "kind=action"); len(got.Hits) != 0 {
		t.Fatalf("read-only token must not be offered Restart: %v", got.refs())
	}
	if got = doSearch(t, rt, reader, "logs api", "kind=action"); len(got.Hits) == 0 {
		t.Fatal("read-only token may open logs")
	}
}

func TestSearch_ScopedTokenSeesOnlyItsApps(t *testing.T) {
	rt, db := newTestRouter(t)
	seedSearchApps(t, db, "mine", "mine-worker", "theirs")
	tok := seedMatrixToken(t, db, "scoped", []string{AbilityRead})
	attachTestPolicy(t, db, "not-theirs", "Deny", "*", "app:theirs", store.PrincipalTypeToken, "scoped")
	attachTestPolicy(t, db, "not-worker", "Deny", "*", "app:mine-worker", store.PrincipalTypeToken, "scoped")

	got := doSearch(t, rt, tok, "kind:app")
	if strings.Join(got.refs(), ",") != "app:mine" {
		t.Fatalf("scoped token saw %v", got.refs())
	}
	for _, q := range []string{"theirs", "kind:app", "app:theirs", "kind:domain theirs"} {
		for _, r := range doSearch(t, rt, tok, q).refs() {
			if strings.Contains(r, "theirs") || strings.Contains(r, "worker") {
				t.Fatalf("query %q leaked %s", q, r)
			}
		}
	}
}

func TestSearch_NodesRequireRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	if err := db.SaveNode(context.Background(), store.Node{ID: "n1", Name: "edge-one", Address: "10.0.0.1", Status: store.NodeStatusOnline}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	reader := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	if got := doSearch(t, rt, root, "edge-one"); len(got.Hits) != 1 {
		t.Fatalf("root should find the node: %v", got.refs())
	}
	if got := doSearch(t, rt, reader, "edge-one"); len(got.Hits) != 0 {
		t.Fatalf("reader must not see nodes: %v", got.refs())
	}
}

func TestSearch_RoutesRequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	for _, p := range []string{"/api/v1/search?q=a", "/api/v1/search/suggest?q=a", "/api/v1/search/saved"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s anonymous = %d", p, rec.Code)
		}
	}
}

func TestSearch_SavedAndUsageViaAPI(t *testing.T) {
	rt, db := newTestRouter(t)
	seedSearchApps(t, db, "alpha")
	cookie := loginTestSession(t, rt, db)

	do := func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
		return rec
	}
	if rec := do(http.MethodPost, "/api/v1/search/saved", `{"name":"prod","query":"env:prod alpha"}`); rec.Code != http.StatusCreated {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}
	rec := do(http.MethodGet, "/api/v1/search/saved", "")
	var saved struct {
		Saved []struct{ ID, Name string } `json:"saved"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil || len(saved.Saved) != 1 {
		t.Fatalf("list = %s", rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/v1/search/usage", `{"ref":"app:alpha"}`); rec.Code != http.StatusNotFound && rec.Code != http.StatusNoContent {
		t.Fatalf("usage = %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/search/usage", `{"ref":"app:does-not-exist"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown ref usage = %d", rec.Code)
	}
	if rec := do(http.MethodDelete, "/api/v1/search/saved/"+saved.Saved[0].ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/api/v1/search?q=a&kind=bogus", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad kind = %d", rec.Code)
	}
}

// TestSearchProvidersAllGoThroughAuthz fails when a provider's docs lack the
// resource fields the hook needs, or when a kind returns a hit to a caller
// whose authorizer refuses everything. A new provider that read the index
// directly or skipped Doc.Prefix would trip it.
func TestSearchProvidersAllGoThroughAuthz(t *testing.T) {
	rt, db := newTestRouter(t)
	seedSearchApps(t, db, "api")
	svc := rt.searchService()
	if svc == nil {
		t.Fatal("no search service")
	}
	ctx := context.Background()
	for _, p := range svc.Providers() {
		docs, err := p.Docs(ctx)
		if err != nil {
			t.Fatalf("provider %s: %v", p.Kind(), err)
		}
		for _, d := range docs {
			if err := d.Validate(); err != nil {
				t.Errorf("provider %s: %v", p.Kind(), err)
			}
			if d.Kind != p.Kind() {
				t.Errorf("provider %s emitted a %s doc", p.Kind(), d.Kind)
			}
		}
	}
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	allowAuthz := &countAuthz{}
	allow := search.Caller{PrincipalID: "t:1", Authz: allowAuthz}
	deny := search.Caller{PrincipalID: "t:1", Authz: &countAuthz{deny: true}}
	for _, p := range svc.Providers() {
		kind := p.Kind()
		if kind == search.KindSaved {
			continue
		}
		before := allowAuthz.calls
		r, err := svc.Search(ctx, allow, search.Request{Q: "kind:" + string(kind), Limit: search.MaxPageSize})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Hits) == 0 {
			continue
		}
		if allowAuthz.calls-before < len(r.Hits) {
			t.Errorf("provider %s returned %d hits but the authorizer ran %d times", kind, len(r.Hits), allowAuthz.calls-before)
		}
		d, err := svc.Search(ctx, deny, search.Request{Q: "kind:" + string(kind), Limit: search.MaxPageSize})
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Hits) != 0 {
			t.Errorf("provider %s returned %d hits to a caller that is denied everything", kind, len(d.Hits))
		}
	}
}

type countAuthz struct {
	calls int
	deny  bool
}

func (a *countAuthz) Allowed(string, string, string) bool {
	a.calls++
	return !a.deny
}

// TestSearchIndexHasOneReader keeps every read of search_docs inside
// internal/search, where Service.Search owns the authorization hook.
func TestSearchIndexHasOneReader(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // globbed package files
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "search_docs") {
			t.Errorf("%s reads the search index directly, go through search.Service", f)
		}
	}
}

func TestSearch_ThousandAppsP95(t *testing.T) {
	if testing.Short() {
		t.Skip("1000-app timing test, skipped under -short")
	}
	rt, db := newTestRouter(t)
	for i := range 1000 {
		seedSearchApps(t, db, fmt.Sprintf("svc-%04d", i))
	}
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	doSearch(t, rt, root, "svc")
	queries := []string{"svc-0042", "restart svc-05", "kind:app svc", "logs svc-9", "deploy", "svc"}
	var p95 time.Duration
	for attempt := 0; attempt < 3; attempt++ {
		var times []time.Duration
		for range 10 {
			for _, q := range queries {
				start := time.Now()
				doSearch(t, rt, root, q, "limit=20")
				times = append(times, time.Since(start))
			}
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		p95 = times[len(times)*95/100]
		t.Logf("1000 apps attempt %d: p50=%s p95=%s", attempt, times[len(times)/2], p95)
		if p95 <= 50*time.Millisecond {
			break
		}
	}
	if p95 > 50*time.Millisecond {
		t.Errorf("p95 = %s, want under 50ms", p95)
	}
}
