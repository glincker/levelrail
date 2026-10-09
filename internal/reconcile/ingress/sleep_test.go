package ingress

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSleepSource struct{ rows []store.AppSleep }

func (f fakeSleepSource) ListAppSleep(context.Context) ([]store.AppSleep, error) { return f.rows, nil }

func TestController_Sleep_SleepingAppGetsWakeRoute(t *testing.T) {
	svc := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Replicas: 1, Suspended: true, Domains: []string{"web.example.com"}}
	applier := &fakeApplier{}
	src := fakeSleepSource{rows: []store.AppSleep{{ServiceName: "web", IdleMinutes: 30, Sleeping: true}}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, newFakeRuntime(), applier, WithLogger(discardLogger()), WithAppWake(src, "127.0.0.1:8080", "tok"))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	routes := applier.routes(t)
	if len(routes) != 1 || len(routes[0].Handle) != 2 {
		t.Fatalf("routes = %+v, want one wake route", routes)
	}
	if rw, ok := routes[0].Handle[0].(ingress.RewriteHandler); !ok || rw.URI != ingress.WakePath {
		t.Errorf("first handler = %+v", routes[0].Handle[0])
	}
	rp, ok := routes[0].Handle[1].(ingress.ReverseProxyHandler)
	if !ok || rp.Upstreams[0].Dial != "127.0.0.1:8080" {
		t.Fatalf("second handler = %+v", routes[0].Handle[1])
	}
	set := rp.Headers.Request.Set
	if set[ingress.WakeTokenHeader][0] != "tok" || set[ingress.WakeAppHeader][0] != "web" {
		t.Errorf("headers = %+v", set)
	}
}

func TestController_Sleep_RunningAppKeepsNormalRoute(t *testing.T) {
	svc := lbService(1)
	rt := newFakeRuntime()
	rt.seedRunning(replicaName(svc, 0), 30000)
	applier := &fakeApplier{}
	src := fakeSleepSource{rows: []store.AppSleep{{ServiceName: "web", IdleMinutes: 30, Sleeping: true}}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithAppWake(src, "127.0.0.1:8080", "tok"))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rp := proxyHandler(t, applier); rp.Upstreams[0].Dial != "127.0.0.1:30000" {
		t.Errorf("a started app must route to its container even if the sleeping flag is stale: %+v", rp.Upstreams)
	}
}
