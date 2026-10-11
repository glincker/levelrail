package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// memConditions stands in for the engine's condition store between passes.
type memConditions struct {
	mu   sync.Mutex
	byID map[string][]reconcile.Condition
}

func (m *memConditions) GetConditions(_ context.Context, name string) ([]reconcile.Condition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byID[name], nil
}

func (m *memConditions) put(name string, conds []reconcile.Condition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[name] = conds
}

// TestComposeStack_HalfSucceeded_ServiceTwoOfFour: service 2 of 4 (cache)
// fails to create. db converges, api and web stay blocked behind it with
// their own reasons, and once the fault clears plain repeated passes
// converge every service, in dependency order, with nothing else changed.
func TestComposeStack_HalfSucceeded_ServiceTwoOfFour(t *testing.T) {
	backend := alwaysHealthy()
	defer backend.Close()

	rt := &createErrForPrefixRuntime{
		fakeRuntime: newFakeRuntime(serverPort(t, backend)),
		prefix:      "stack-cache-",
		err:         errors.New("pull access denied"),
	}
	conds := &memConditions{byID: map[string][]reconcile.Condition{}}
	services := []store.DesiredService{
		{Name: "stack-db", AppID: "stack", Image: "postgres:16", Port: 5432},
		{Name: "stack-cache", AppID: "stack", Image: "redis:7", Port: 6379},
		{Name: "stack-api", AppID: "stack", Image: "api:v1", Port: 80, DependsOn: []string{"db", "cache"}, DependsOnHealthy: []string{"db"}},
		{Name: "stack-web", AppID: "stack", Image: "web:v1", Port: 80, DependsOn: []string{"api"}},
	}
	ctrls := make([]*Controller, len(services))
	for i := range services {
		svc := services[i]
		ctrls[i] = New(svc.Name, &fakeStore{svc: &svc}, rt, WithConditionReader(conds))
	}
	pass := func() map[string]reconcile.Condition {
		out := map[string]reconcile.Condition{}
		for _, c := range ctrls {
			res, _ := c.Reconcile(context.Background())
			conds.put(c.Name(), res.Conditions)
			out[c.serviceName] = conditionOf(t, res)
		}
		return out
	}

	got := pass()
	want := map[string]struct {
		status reconcile.ConditionStatus
		reason string
	}{
		"stack-db":    {reconcile.ConditionTrue, ""},
		"stack-cache": {reconcile.ConditionFalse, ""},
		"stack-api":   {reconcile.ConditionUnknown, "WaitingForDependency"},
		"stack-web":   {reconcile.ConditionUnknown, "WaitingForDependency"},
	}
	for name, w := range want {
		c := got[name]
		if c.Status != w.status || (w.reason != "" && c.Reason != w.reason) {
			t.Errorf("after failed pass, %s condition = %+v, want status %s reason %q", name, c, w.status, w.reason)
		}
	}

	rt.prefix = "never-matches-"
	for i := 0; i < 4; i++ {
		got = pass()
	}
	for _, svc := range services {
		if c := got[svc.Name]; c.Status != reconcile.ConditionTrue {
			t.Errorf("after recovery, %s condition = %+v, want Ready=True", svc.Name, c)
		}
	}
}

// TestComposeStack_ServiceHealthyWaitsForReady covers condition:
// service_healthy: a running but not-yet-Ready dependency still blocks.
func TestComposeStack_ServiceHealthyWaitsForReady(t *testing.T) {
	tests := []struct {
		name       string
		dbCond     []reconcile.Condition
		withReader bool
		wantReason string
	}{
		{"db running, not ready", []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse}}, true, "WaitingForDependencyHealthy"},
		{"db running, no condition yet", nil, true, "WaitingForDependencyHealthy"},
		{"db ready", []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue}}, true, ""},
		{"no reader degrades to start order", nil, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := alwaysHealthy()
			defer backend.Close()
			rt := newFakeRuntime(serverPort(t, backend))
			rt.seed(replicaContainerName("stack-db", "postgres:16", "", 0), true)
			conds := &memConditions{byID: map[string][]reconcile.Condition{ControllerName("stack-db"): tt.dbCond}}
			svc := &store.DesiredService{Name: "stack-api", AppID: "stack", Image: "api:v1", Port: 80, DependsOn: []string{"db"}, DependsOnHealthy: []string{"db"}}
			var opts []Option
			if tt.withReader {
				opts = append(opts, WithConditionReader(conds))
			}
			res, err := New("stack-api", &fakeStore{svc: svc}, rt, opts...).Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			cond := conditionOf(t, res)
			if tt.wantReason != "" && cond.Reason != tt.wantReason {
				t.Errorf("condition = %+v, want reason %q", cond, tt.wantReason)
			}
			if tt.wantReason == "" && cond.Status != reconcile.ConditionTrue {
				t.Errorf("condition = %+v, want Ready=True", cond)
			}
		})
	}
}
