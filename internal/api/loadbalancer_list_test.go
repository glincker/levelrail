package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLoadBalancerListRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodGet, "/api/v1/loadbalancers"}})
}

func TestSummarizeLoadBalancer(t *testing.T) {
	up := func(running, draining bool) loadbalancer.UpstreamObservation {
		return loadbalancer.UpstreamObservation{Running: running, Draining: draining}
	}
	tests := []struct {
		name        string
		obs         *loadbalancer.Observation
		wantState   string
		wantHealthy int
		wantTotal   int
	}{
		{"no observation", nil, lbStateNone, 0, 0},
		{"no upstreams", &loadbalancer.Observation{Ready: true}, lbStateNone, 0, 0},
		{"all healthy", &loadbalancer.Observation{Ready: true, Upstreams: []loadbalancer.UpstreamObservation{up(true, false), up(true, false)}}, lbStateBalancing, 2, 2},
		{"one down", &loadbalancer.Observation{Ready: true, Upstreams: []loadbalancer.UpstreamObservation{up(true, false), up(false, false)}}, lbStateDegraded, 1, 2},
		{"draining counts unhealthy", &loadbalancer.Observation{Ready: true, Upstreams: []loadbalancer.UpstreamObservation{up(true, false), up(true, true)}}, lbStateDegraded, 1, 2},
		{"not ready", &loadbalancer.Observation{Ready: false, Upstreams: []loadbalancer.UpstreamObservation{up(true, false)}}, lbStateDegraded, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeLoadBalancer(store.LoadBalancerRow{Service: "web"}, loadbalancer.Config{}, tt.obs)
			if got.State != tt.wantState || got.UpstreamsHealthy != tt.wantHealthy || got.UpstreamsTotal != tt.wantTotal {
				t.Fatalf("got %+v, want state %s healthy %d total %d", got, tt.wantState, tt.wantHealthy, tt.wantTotal)
			}
		})
	}
}

func TestLoadBalancerList_FilterPageAndState(t *testing.T) {
	reg := loadbalancer.NewRegistry()
	rt, db, cookie := newLBRouter(t, reg, nil)
	ctx := context.Background()
	for _, name := range []string{"api", "web", "worker"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:v1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetServiceLoadBalancer(ctx, name, `{"algorithm":"least_conn"}`); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "plain", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	reg.Record(loadbalancer.Observation{Service: "web", Ready: true, ObservedAt: time.Now(), Upstreams: []loadbalancer.UpstreamObservation{{Running: true}, {Running: false}}})
	reg.Record(loadbalancer.Observation{Service: "api", Ready: true, ObservedAt: time.Now(), Upstreams: []loadbalancer.UpstreamObservation{{Running: true}}})

	tests := []struct {
		name, query string
		wantApps    []string
		wantTotal   int
	}{
		{"all sorted, unconfigured excluded", "", []string{"api", "web", "worker"}, 3},
		{"search", "?q=WEB", []string{"web"}, 1},
		{"degraded", "?state=degraded", []string{"web"}, 1},
		{"balancing", "?state=balancing", []string{"api"}, 1},
		{"none", "?state=none", []string{"worker"}, 1},
		{"page", "?limit=1&offset=1", []string{"web"}, 3},
		{"offset past end", "?offset=99", []string{}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/loadbalancers"+tt.query, "")
			var res loadBalancerListResponse
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			var apps []string
			for _, it := range res.Items {
				apps = append(apps, it.App)
			}
			if fmt.Sprint(apps) != fmt.Sprint(tt.wantApps) || res.Total != tt.wantTotal {
				t.Fatalf("apps = %v total = %d, want %v total %d", apps, res.Total, tt.wantApps, tt.wantTotal)
			}
		})
	}
}

func TestLoadBalancerList_NotConfiguredReturns501(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/loadbalancers", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestLoadBalancerList_RespectsIAMDeny(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithLoadBalancers(db, nil, nil))
	bootstrapTestAdmin(t, db)
	ctx := context.Background()
	for _, name := range []string{"prod-web", "staging-web"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:v1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetServiceLoadBalancer(ctx, name, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-prod-read", "Deny", AbilityRead, "app:prod-web", store.PrincipalTypeUser, reader.ID)
	cookie := sessionCookieForTest(t, rt, reader.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/loadbalancers", ""))
	var res loadBalancerListResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(res.Items) != 1 || res.Items[0].App != "staging-web" {
		t.Fatalf("items = %+v, want only staging-web", res.Items)
	}
}
