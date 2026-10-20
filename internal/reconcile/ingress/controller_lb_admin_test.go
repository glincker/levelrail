package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeLBAdminSource struct {
	fakeLBSource
	states map[string]map[int]string
	err    error
}

func (f fakeLBAdminSource) ListLBUpstreamAdminStates(context.Context) (map[string]map[int]string, error) {
	return f.states, f.err
}

func newAdminController(t *testing.T, replicas int, states map[string]map[int]string, applier *fakeApplier, reg *loadbalancer.Registry) *Controller {
	t.Helper()
	svc := lbService(replicas)
	rt := newFakeRuntime()
	for i := 0; i < replicas; i++ {
		rt.seedRunning(replicaName(svc, i), 30000+i)
	}
	src := fakeLBAdminSource{fakeLBSource: fakeLBSource{configs: map[string]string{"web": `{}`}}, states: states}
	return New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(src, reg))
}

func TestController_LoadBalancer_AdminStateHoldsUpstreamsOutOfPool(t *testing.T) {
	tests := []struct {
		name      string
		states    map[int]string
		wantDials []string
		reason    string
	}{
		{"disabled leaves the pool", map[int]string{1: loadbalancer.AdminDisabled}, []string{"127.0.0.1:30000", "127.0.0.1:30002"}, "UpstreamsDegraded"},
		{"draining leaves the pool", map[int]string{0: loadbalancer.AdminDraining}, []string{"127.0.0.1:30001", "127.0.0.1:30002"}, "UpstreamsDegraded"},
		{"active row is a no-op", map[int]string{2: loadbalancer.AdminActive}, []string{"127.0.0.1:30000", "127.0.0.1:30001", "127.0.0.1:30002"}, "Balancing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applier := &fakeApplier{}
			reg := loadbalancer.NewRegistry()
			c := newAdminController(t, 3, map[string]map[int]string{"web": tt.states}, applier, reg)
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			rp := proxyHandler(t, applier)
			var dials []string
			for _, u := range rp.Upstreams {
				dials = append(dials, u.Dial)
			}
			if len(dials) != len(tt.wantDials) {
				t.Fatalf("pool = %v, want %v", dials, tt.wantDials)
			}
			for i := range dials {
				if dials[i] != tt.wantDials[i] {
					t.Fatalf("pool = %v, want %v", dials, tt.wantDials)
				}
			}
			if cond := lbConditionOf(t, res); cond.Reason != tt.reason {
				t.Errorf("condition = %+v, want reason %s", cond, tt.reason)
			}
			obs, _ := reg.Get("web")
			for replica, state := range tt.states {
				if obs.Upstreams[replica].AdminState != state {
					t.Errorf("observation replica %d admin state = %q, want %q", replica, obs.Upstreams[replica].AdminState, state)
				}
			}
		})
	}
}

func TestController_LoadBalancer_AdminState_HalfSucceededApplyConverges(t *testing.T) {
	applier := &fakeApplier{applyErr: errors.New("caddy rejected config")}
	reg := loadbalancer.NewRegistry()
	c := newAdminController(t, 2, map[string]map[int]string{"web": {0: loadbalancer.AdminDisabled}}, applier, reg)

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("want apply error")
	}
	if _, ok := reg.Get("web"); ok {
		t.Fatal("observation recorded although the pool was never applied")
	}

	applier.applyErr = nil
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rp := proxyHandler(t, applier); len(rp.Upstreams) != 1 || rp.Upstreams[0].Dial != "127.0.0.1:30001" {
		t.Errorf("pool after retry = %+v, want only replica 1", rp.Upstreams)
	}
	if cond := lbConditionOf(t, res); cond.Reason != "UpstreamsDegraded" {
		t.Errorf("condition = %+v", cond)
	}
	if again, err := c.Reconcile(context.Background()); err != nil || lbConditionOf(t, again).Reason != "UpstreamsDegraded" {
		t.Errorf("third pass must be idempotent, err=%v", err)
	}
}

func TestController_LoadBalancer_AllHeldOutLeavesServiceUnrouted(t *testing.T) {
	applier := &fakeApplier{}
	reg := loadbalancer.NewRegistry()
	c := newAdminController(t, 1, map[string]map[int]string{"web": {0: loadbalancer.AdminDisabled}}, applier, reg)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cond := lbConditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "NoUpstreams" {
		t.Errorf("condition = %+v", cond)
	}
}

func TestController_LoadBalancer_AdminStateStoreErrorIsNotReady(t *testing.T) {
	svc := lbService(1)
	src := fakeLBAdminSource{fakeLBSource: fakeLBSource{configs: map[string]string{"web": `{}`}}, err: errors.New("db down")}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, newFakeRuntime(), &fakeApplier{}, WithLogger(discardLogger()), WithLoadBalancers(src, loadbalancer.NewRegistry()))
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("a failed admin state read must not fail open")
	}
	if cond := conditionOf(t, res); cond.Reason != "StoreError" {
		t.Errorf("condition = %+v", cond)
	}
}
