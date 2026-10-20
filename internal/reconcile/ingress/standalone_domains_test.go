package ingress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type standaloneFakeStore struct {
	*fakeStore
	standalone    []store.StandaloneDomain
	standaloneErr error
}

func (f *standaloneFakeStore) ListStandaloneDomains(context.Context) ([]store.StandaloneDomain, error) {
	return f.standalone, f.standaloneErr
}

func routeHosts(routes []ingress.Route) map[string]string {
	out := map[string]string{}
	for _, r := range routes {
		raw, _ := json.Marshal(r.Handle)
		for _, m := range r.Match {
			for _, h := range m.Host {
				out[h] = string(raw)
			}
		}
	}
	return out
}

func standaloneCondition(res reconcile.Result) *reconcile.Condition {
	for i := range res.Conditions {
		if res.Conditions[i].Type == conditionStandaloneDomains {
			return &res.Conditions[i]
		}
	}
	return nil
}

func TestStandaloneDomains_RouteEveryTargetKind(t *testing.T) {
	web := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(web.Name, web.Image, ""), 34567)
	rt.containers["redis-ui"] = &docker.ContainerState{Name: "redis-ui", Running: true, Networks: []docker.ContainerNetwork{{Name: "bridge", IPAddress: "172.18.0.4"}}}
	st := &standaloneFakeStore{fakeStore: &fakeStore{services: []store.DesiredService{web}}, standalone: []store.StandaloneDomain{
		{Domain: "port.example.com", Target: "127.0.0.1:3000", WebSocket: true},
		{Domain: "lan.example.com", Target: "10.0.0.5:8443", UpstreamTLS: true, TLSSkipVerify: true, WebSocket: true, StripPrefix: "/app"},
		{Domain: "url.example.com", Target: "https://203.0.113.9:8443/base", WebSocket: true},
		{Domain: "app.example.com", Target: "app:web", WebSocket: false},
		{Domain: "ui.example.com", Target: "container:redis-ui:8081", WebSocket: true, HostHeader: "upstream"},
	}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if cond := standaloneCondition(res); cond != nil {
		t.Fatalf("unexpected unrouted condition: %+v", cond)
	}
	hosts := routeHosts(applier.routes(t))
	want := map[string][]string{
		"port.example.com": {`"dial":"127.0.0.1:3000"`},
		"lan.example.com":  {`"dial":"10.0.0.5:8443"`, `"insecure_skip_verify":true`, `"strip_path_prefix":"/app"`},
		"url.example.com":  {`"dial":"203.0.113.9:8443"`, `"tls":{}`, `"Host":["203.0.113.9:8443"]`, `"uri":"/base{http.request.uri}"`},
		"app.example.com":  {`"dial":"127.0.0.1:34567"`, `"delete":["Upgrade"]`},
		"ui.example.com":   {`"dial":"172.18.0.4:8081"`, `"Host":["{http.reverse_proxy.upstream.hostport}"]`},
	}
	for host, needles := range want {
		got, ok := hosts[host]
		if !ok {
			t.Errorf("%s has no route", host)
			continue
		}
		for _, n := range needles {
			if !strings.Contains(got, n) {
				t.Errorf("%s handle %s missing %s", host, got, n)
			}
		}
	}
}

// One unavailable target must not stop the others routing, and the
// condition names it; once the target exists the next pass routes it.
func TestStandaloneDomains_HalfSucceeded(t *testing.T) {
	rt := newFakeRuntime()
	st := &standaloneFakeStore{fakeStore: &fakeStore{}, standalone: []store.StandaloneDomain{
		{Domain: "ok.example.com", Target: "3000", WebSocket: true},
		{Domain: "later.example.com", Target: "container:late:80", WebSocket: true},
		{Domain: "meta.example.com", Target: "169.254.169.254:80", WebSocket: true},
	}}
	applier := &fakeApplier{applyErr: errors.New("caddy rejected config")}
	c := New(st, rt, applier, WithLogger(discardLogger()))
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("first pass should surface the apply failure")
	}
	applier.applyErr = nil
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	hosts := routeHosts(applier.routes(t))
	if _, ok := hosts["ok.example.com"]; !ok {
		t.Fatal("ok.example.com must route even though siblings cannot")
	}
	if _, ok := hosts["later.example.com"]; ok {
		t.Fatal("later.example.com has no running container and must not route")
	}
	if _, ok := hosts["meta.example.com"]; ok {
		t.Fatal("a metadata target must never route")
	}
	cond := standaloneCondition(res)
	if cond == nil || !strings.Contains(cond.Message, "later.example.com") || !strings.Contains(cond.Message, "meta.example.com") {
		t.Fatalf("condition = %+v, want both unrouted domains named", cond)
	}

	rt.containers["late"] = &docker.ContainerState{Name: "late", Running: true, Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 41000}}}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := routeHosts(applier.routes(t))["later.example.com"]; !strings.Contains(got, "127.0.0.1:41000") {
		t.Fatalf("later.example.com handle = %s, want the published port", got)
	}
	first := routeHosts(applier.routes(t))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if again := routeHosts(applier.routes(t)); len(again) != len(first) || again["ok.example.com"] != first["ok.example.com"] {
		t.Fatal("a repeat pass must produce the same routes")
	}
}

func TestStandaloneDomains_FeaturesAndConflicts(t *testing.T) {
	web := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"taken.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(web.Name, web.Image, ""), 34567)
	st := &standaloneFakeStore{
		fakeStore: &fakeStore{
			services:    []store.DesiredService{web},
			maintenance: []string{"down.example.com"},
			redirects:   []store.DomainRedirect{{Domain: "moved.example.com", TargetURL: "https://new.example.com", StatusCode: 301}},
			waf:         []store.DomainWAF{{Domain: "waf.example.com", RateLimitRPS: 5}},
		},
		standalone: []store.StandaloneDomain{
			{Domain: "taken.example.com", Target: "3000"},
			{Domain: "down.example.com", Target: "3001"},
			{Domain: "moved.example.com", Target: "3002"},
			{Domain: "waf.example.com", Target: "3003"},
		},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	hosts := routeHosts(applier.routes(t))
	if !strings.Contains(hosts["taken.example.com"], "34567") {
		t.Errorf("the app must keep its domain, got %s", hosts["taken.example.com"])
	}
	if strings.Contains(hosts["down.example.com"], "3001") {
		t.Errorf("maintenance must win over the target, got %s", hosts["down.example.com"])
	}
	if !strings.Contains(hosts["moved.example.com"], "new.example.com") {
		t.Errorf("redirect not applied, got %s", hosts["moved.example.com"])
	}
	if !strings.Contains(hosts["waf.example.com"], "rate_limit") || !strings.Contains(hosts["waf.example.com"], "127.0.0.1:3003") {
		t.Errorf("WAF rate limit not applied, got %s", hosts["waf.example.com"])
	}
	if cond := standaloneCondition(res); cond == nil || !strings.Contains(cond.Message, "taken.example.com") {
		t.Errorf("conflict should be reported, got %+v", cond)
	}
}

func TestStandaloneDomains_DeleteRemovesRoute(t *testing.T) {
	st := &standaloneFakeStore{fakeStore: &fakeStore{}, standalone: []store.StandaloneDomain{{Domain: "test.example.com", Target: "127.0.0.1:3000", WebSocket: true}}}
	applier := &fakeApplier{}
	c := New(st, newFakeRuntime(), applier, WithLogger(discardLogger()))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := routeHosts(applier.routes(t))["test.example.com"]; !ok {
		t.Fatal("route missing after create")
	}
	st.standalone = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := routeHosts(applier.routes(t))["test.example.com"]; ok {
		t.Fatal("route still present after delete")
	}
}

func TestStandaloneDomains_StoreErrorFailsPass(t *testing.T) {
	st := &standaloneFakeStore{fakeStore: &fakeStore{}, standaloneErr: errors.New("db down")}
	c := New(st, newFakeRuntime(), &fakeApplier{}, WithLogger(discardLogger()))
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("a store error must fail the pass rather than drop every standalone route")
	}
}
