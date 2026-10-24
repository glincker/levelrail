package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/search"
	"github.com/GLINCKER/levelrail/internal/store"
)

func rawSearchBody(t *testing.T, rt *Router, tok, target string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// The read-pool wiring and the candidate cache must return the same bytes as
// a fresh service reading through the writer connection, for every caller.
func TestSearch_CachedAndPooledMatchFreshService(t *testing.T) {
	rt, db := newTestRouter(t)
	for i := range 40 {
		seedApp(t, db, fmt.Sprintf("svc-%02d", i))
	}
	seedApp(t, db, "secret-api")
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	deployer := seedMatrixToken(t, db, "deployer", []string{AbilityRead, AbilityDeploy})
	reader := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-secret", "Deny", "*", "app:secret-api", store.PrincipalTypeToken, "deployer")
	attachTestPolicy(t, db, "deny-svc-1", "Deny", "*", "app:svc-1*", store.PrincipalTypeToken, "reader")

	shared := rt.searchService()
	if shared == nil {
		t.Fatal("search unavailable")
	}
	targets := []string{}
	for _, q := range []string{"svc", "svc-1", "restart svc", "secret", "app:svc-2 deploy", "kind:app svc-0", "svc-zz", "se", "nomatchxyz"} {
		targets = append(targets, "/api/v1/search?q="+url.QueryEscape(q)+"&limit=7")
	}
	targets = append(targets, "/api/v1/search?q=svc&limit=5&cursor=5", "/api/v1/search?q=svc&kind=action", "/api/v1/search/suggest?q=sv")

	for name, tok := range map[string]string{"root": root, "deployer": deployer, "reader": reader} {
		for _, target := range targets {
			rt.searchSvc = search.New(db, rt.logger, rt.searchProviders()...)
			want := rawSearchBody(t, rt, tok, target)
			rt.searchSvc = shared
			cold := rawSearchBody(t, rt, tok, target)
			warm := rawSearchBody(t, rt, tok, target)
			if cold != want || warm != want {
				t.Errorf("%s %s differs\nwant %s\ncold %s\nwarm %s", name, target, want, cold, warm)
			}
		}
	}
	if got := rawSearchBody(t, rt, root, "/api/v1/search?q=secret-api"); !strings.Contains(got, "app:secret-api") {
		t.Errorf("root should see the app: %s", got)
	}
	if got := rawSearchBody(t, rt, deployer, "/api/v1/search?q=secret-api"); strings.Contains(got, "app:secret-api") {
		t.Errorf("restricted caller saw denied app: %s", got)
	}
}

func TestSearch_CandidateCacheDropsOnReconcile(t *testing.T) {
	rt, db := newTestRouter(t)
	seedApp(t, db, "alpha-one")
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	if got := rawSearchBody(t, rt, root, "/api/v1/search?q=alpha&kind=app"); !strings.Contains(got, "alpha-one") {
		t.Fatalf("missing alpha-one: %s", got)
	}
	seedApp(t, db, "alpha-two")
	if err := rt.searchService().Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rawSearchBody(t, rt, root, "/api/v1/search?q=alpha&kind=app"); !strings.Contains(got, "alpha-two") {
		t.Fatalf("stale candidates after reconcile: %s", got)
	}
}
