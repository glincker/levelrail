package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeLBSource struct {
	configs map[string]string
	err     error
}

func (f fakeLBSource) ListServiceLoadBalancers(context.Context) (map[string]string, error) {
	return f.configs, f.err
}

// prefixRuntime adds ListByPrefix results to fakeRuntime.
type prefixRuntime struct {
	*fakeRuntime
	byPrefix []docker.ContainerState
}

func (p *prefixRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return p.byPrefix, nil
}

type fakeNodes struct {
	rt   docker.Runtime
	host string
	err  error
}

func (f fakeNodes) UpstreamHost(context.Context, string) (docker.Runtime, string, error) {
	return f.rt, f.host, f.err
}

func lbService(replicas int) store.DesiredService {
	return store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Replicas: replicas, Domains: []string{"web.example.com"}}
}

func replicaName(svc store.DesiredService, i int) string {
	return application.ReplicaContainerName(svc.Name, svc.Image, svc.RestartNonce, i)
}

func proxyHandler(t *testing.T, applier *fakeApplier) ingress.ReverseProxyHandler {
	t.Helper()
	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(routes))
	}
	for _, h := range routes[0].Handle {
		if rp, ok := h.(ingress.ReverseProxyHandler); ok {
			return rp
		}
	}
	t.Fatalf("no reverse_proxy handler in %+v", routes[0].Handle)
	return ingress.ReverseProxyHandler{}
}

func lbConditionOf(t *testing.T, res reconcile.Result) reconcile.Condition {
	t.Helper()
	for _, c := range res.Conditions {
		if c.Type == "LoadBalancer" {
			return c
		}
	}
	t.Fatalf("no LoadBalancer condition in %+v", res.Conditions)
	return reconcile.Condition{}
}

func TestController_LoadBalancer_PoolsAllReplicas(t *testing.T) {
	svc := lbService(3)
	rt := newFakeRuntime()
	for i := 0; i < 3; i++ {
		rt.seedRunning(replicaName(svc, i), 30000+i)
	}
	applier := &fakeApplier{}
	reg := loadbalancer.NewRegistry()
	src := fakeLBSource{configs: map[string]string{"web": `{"algorithm":"least_conn","active_health":{"path":"/healthz"}}`}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(src, reg))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rp := proxyHandler(t, applier)
	if len(rp.Upstreams) != 3 || rp.Upstreams[2].Dial != "127.0.0.1:30002" {
		t.Errorf("upstreams = %+v, want 3 replicas", rp.Upstreams)
	}
	if rp.LoadBalancing == nil || rp.LoadBalancing.SelectionPolicy.Policy != "least_conn" || rp.HealthChecks.Active.URI != "/healthz" {
		t.Errorf("load balancing = %+v health = %+v", rp.LoadBalancing, rp.HealthChecks)
	}
	if cond := lbConditionOf(t, res); cond.Status != reconcile.ConditionTrue || cond.Reason != "Balancing" {
		t.Errorf("condition = %+v", cond)
	}
	if obs, ok := reg.Get("web"); !ok || !obs.Ready || len(obs.Upstreams) != 3 {
		t.Errorf("observation = %+v ok=%v", obs, ok)
	}
}

func TestController_LoadBalancer_HalfSucceeded_OneReplicaInspectFails(t *testing.T) {
	svc := lbService(3)
	rt := newFakeRuntime()
	rt.seedRunning(replicaName(svc, 0), 30000)
	rt.seedInspectErr(replicaName(svc, 1), errors.New("docker hiccup"))
	rt.seedStopped(replicaName(svc, 2))
	applier := &fakeApplier{}
	reg := loadbalancer.NewRegistry()
	src := fakeLBSource{configs: map[string]string{"web": `{}`}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(src, reg))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("a partial pool must still apply, got %v", err)
	}
	if rp := proxyHandler(t, applier); len(rp.Upstreams) != 1 {
		t.Errorf("upstreams = %+v, want only the healthy replica", rp.Upstreams)
	}
	if cond := lbConditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason != "UpstreamsDegraded" {
		t.Errorf("condition = %+v, want False/UpstreamsDegraded", cond)
	}
	obs, _ := reg.Get("web")
	if len(obs.Upstreams) != 3 || obs.Upstreams[1].Note != "inspect failed" || obs.Upstreams[2].Note != "container not running" {
		t.Errorf("observation should keep every replica with its reason: %+v", obs.Upstreams)
	}
}

func TestController_LoadBalancer_ApplyFailureRecordsNothing(t *testing.T) {
	svc := lbService(1)
	rt := newFakeRuntime()
	rt.seedRunning(replicaName(svc, 0), 30000)
	applier := &fakeApplier{applyErr: errors.New("caddy rejected config")}
	reg := loadbalancer.NewRegistry()
	src := fakeLBSource{configs: map[string]string{"web": `{}`}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(src, reg))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("want apply error")
	}
	if _, ok := reg.Get("web"); ok {
		t.Error("observation recorded although the config was never applied")
	}
	applier.applyErr = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("web"); !ok {
		t.Error("retry must record the observation")
	}
}

func TestController_LoadBalancer_NoUpstreamsLeavesServiceUnrouted(t *testing.T) {
	svc := lbService(2)
	applier := &fakeApplier{}
	src := fakeLBSource{configs: map[string]string{"web": `{}`}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, newFakeRuntime(), applier, WithLogger(discardLogger()), WithLoadBalancers(src, nil))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(applier.routes(t)) != 0 {
		t.Error("service with no upstream must not be routed")
	}
	if cond := lbConditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason != "NoUpstreams" {
		t.Errorf("condition = %+v", cond)
	}
}

func TestController_LoadBalancer_ServesPreviousReleaseDuringCutover(t *testing.T) {
	svc := lbService(1)
	oldName := application.ReplicaContainerName(svc.Name, "img:v0", "", 0)
	base := newFakeRuntime()
	rt := &prefixRuntime{fakeRuntime: base, byPrefix: []docker.ContainerState{
		{Name: oldName, Running: true, Ports: []docker.PortBinding{{HostPort: 31000}}},
		{Name: "web-worker-abcdef123456", Running: true, Ports: []docker.PortBinding{{HostPort: 32000}}},
	}}
	applier := &fakeApplier{}
	reg := loadbalancer.NewRegistry()
	src := fakeLBSource{configs: map[string]string{"web": `{}`}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(src, reg))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rp := proxyHandler(t, applier)
	if len(rp.Upstreams) != 1 || rp.Upstreams[0].Dial != "127.0.0.1:31000" {
		t.Errorf("upstreams = %+v, want the previous release container only", rp.Upstreams)
	}
	if cond := lbConditionOf(t, res); cond.Reason != "ServingPreviousRelease" || cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v", cond)
	}
}

func TestController_LoadBalancer_RemoteNodeUpstreams(t *testing.T) {
	svc := lbService(2)
	svc.NodeID = "node-2"
	remote := newFakeRuntime()
	remote.containers[replicaName(svc, 0)] = &docker.ContainerState{Running: true, Ports: []docker.PortBinding{{HostPort: 40000, HostIP: "0.0.0.0"}}}
	remote.containers[replicaName(svc, 1)] = &docker.ContainerState{Running: true, Ports: []docker.PortBinding{{HostPort: 40001, HostIP: "127.0.0.1"}}}
	src := fakeLBSource{configs: map[string]string{"web": `{}`}}

	tests := []struct {
		name      string
		nodes     NodeUpstreamResolver
		wantDials []string
	}{
		{"mesh host and loopback-bound replica skipped", fakeNodes{rt: remote, host: "10.44.0.2"}, []string{"10.44.0.2:40000"}},
		{"no resolver leaves remote service unrouted", nil, nil},
		{"resolver error leaves remote service unrouted", fakeNodes{err: errors.New("node offline")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applier := &fakeApplier{}
			opts := []Option{WithLogger(discardLogger()), WithLoadBalancers(src, nil)}
			if tt.nodes != nil {
				opts = append(opts, WithNodeUpstreams(tt.nodes))
			}
			c := New(&fakeStore{services: []store.DesiredService{svc}}, newFakeRuntime(), applier, opts...)
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if tt.wantDials == nil {
				if len(applier.routes(t)) != 0 {
					t.Error("want no route")
				}
				return
			}
			rp := proxyHandler(t, applier)
			if len(rp.Upstreams) != len(tt.wantDials) || rp.Upstreams[0].Dial != tt.wantDials[0] {
				t.Errorf("upstreams = %+v, want %v", rp.Upstreams, tt.wantDials)
			}
		})
	}
}

func TestController_LoadBalancer_BadConfigFallsBackToPlainRoute(t *testing.T) {
	svc := lbService(1)
	rt := newFakeRuntime()
	rt.seedRunning(replicaName(svc, 0), 30000)
	applier := &fakeApplier{}
	src := fakeLBSource{configs: map[string]string{"web": `{"algorithm":"bogus"}`}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(src, nil))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rp := proxyHandler(t, applier); rp.LoadBalancing != nil || len(rp.Upstreams) != 1 {
		t.Errorf("invalid config must not affect the route: %+v", rp)
	}
	for _, cond := range res.Conditions {
		if cond.Type == "LoadBalancer" {
			t.Errorf("unexpected LoadBalancer condition %+v", cond)
		}
	}
}

func TestController_LoadBalancer_StoreErrorIsNotReady(t *testing.T) {
	c := New(&fakeStore{}, newFakeRuntime(), &fakeApplier{}, WithLogger(discardLogger()), WithLoadBalancers(fakeLBSource{err: errors.New("db down")}, nil))
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if cond := conditionOf(t, res); cond.Reason != "StoreError" {
		t.Errorf("condition = %+v", cond)
	}
}

func TestController_LoadBalancer_WeightedSlowStartRamps(t *testing.T) {
	svc := lbService(2)
	rt := newFakeRuntime()
	rt.seedRunning(replicaName(svc, 0), 30000)
	applier := &fakeApplier{}
	reg := loadbalancer.NewRegistry()
	src := fakeLBSource{configs: map[string]string{"web": `{"algorithm":"weighted","weights":[5,5],"slow_start":"1h"}`}}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(src, reg))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt.seedRunning(replicaName(svc, 1), 30001)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rp := proxyHandler(t, applier)
	w := rp.LoadBalancing.SelectionPolicy.Weights
	if len(w) != 2 || w[0] != 5 || w[1] >= 5 {
		t.Errorf("weights = %v, want the new replica ramping below 5", w)
	}
}
