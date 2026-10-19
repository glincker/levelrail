package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const scopedP95Budget = 50 * time.Millisecond

func scopedGet(t *testing.T, rt *Router, cookie *http.Cookie, target string) time.Duration {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	start := time.Now()
	rt.Handler().ServeHTTP(rec, req)
	d := time.Since(start)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body.String())
	}
	return d
}

func percentile(times []time.Duration, p int) time.Duration {
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	return times[len(times)*p/100]
}

// TestSearch_ScopedCallerP95 measures a default-team developer (team scope active) holding 50 IAM
// policies against search and the ranked apps list, at 1000 and 5000 apps.
func TestSearch_ScopedCallerP95(t *testing.T) {
	if testing.Short() {
		t.Skip("scoped p95 timing test, skipped under -short")
	}
	for _, n := range []int{1000, 5000} {
		t.Run(fmt.Sprintf("%d_apps", n), func(t *testing.T) {
			rt, db, _ := newPipelineRouter(t)
			bootstrapTestAdmin(t, db)
			ctx := context.Background()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			for i := range n {
				must(db.SaveDesiredService(ctx, store.DesiredService{Name: fmt.Sprintf("svc-%04d", i), Image: "nginx:1", Port: 80}))
			}
			u := storeUserWithAbilitiesForTest(t, db, "dev@instance.test", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy})
			for i := range 50 {
				attachTestPolicy(t, db, fmt.Sprintf("deny-%02d", i), "Deny", "*", fmt.Sprintf("app:svc-%04d", i*2+1), store.PrincipalTypeUser, u.ID)
			}
			cookie := sessionCookieForTest(t, rt, u.ID)
			scopedGet(t, rt, cookie, "/api/v1/search?q=svc")

			queries := []string{"svc-0042", "restart svc-05", "kind:app svc", "logs svc-9", "deploy", "svc"}
			for _, kind := range []string{"search", "apps list"} {
				var times []time.Duration
				for range 5 {
					for _, q := range queries {
						target := "/api/v1/search?limit=20&q=" + url.QueryEscape(q)
						if kind == "apps list" {
							target = "/api/v1/apps?limit=50&q=" + url.QueryEscape(q)
						}
						times = append(times, scopedGet(t, rt, cookie, target))
					}
				}
				p50, p95 := percentile(times, 50), percentile(times, 95)
				t.Logf("%d apps, scoped, 50 policies, %s: p50=%s p95=%s", n, kind, p50, p95)
				if n == 1000 && p95 > scopedP95Budget {
					t.Errorf("%s p95 = %s at %d apps, want under %s", kind, p95, n, scopedP95Budget)
				}
			}
		})
	}
}
