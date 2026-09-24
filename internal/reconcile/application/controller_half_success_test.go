package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_HalfSuccess_RedeployRecoversOnRetry injects one
// failure into a redeploy (v1 to v2) per strategy, checks what the failed
// pass left behind, then clears it and checks the next pass converges to
// exactly the v2 replica set with no v1 leftovers.
func TestController_Reconcile_HalfSuccess_RedeployRecoversOnRetry(t *testing.T) {
	v1 := func(i int) string { return replicaContainerName("web", "img:v1", "", i) }
	v2 := func(i int) string { return replicaContainerName("web", "img:v2", "", i) }

	tests := []struct {
		name     string
		strategy string
		replicas int
		inject   func(rt *fakeRuntime)
		clear    func(rt *fakeRuntime)
		// wantAfterFail lists the container names that must exist after the failed pass.
		wantAfterFail func() []string
	}{
		{
			name: "blue-green: create fails, old keeps serving", strategy: "blue-green", replicas: 1,
			inject:        func(rt *fakeRuntime) { rt.createErr = errors.New("pull denied") },
			clear:         func(rt *fakeRuntime) { rt.createErr = nil },
			wantAfterFail: func() []string { return []string{v1(0)} },
		},
		{
			name: "blue-green: create ok, start fails, old keeps serving", strategy: "blue-green", replicas: 1,
			inject:        func(rt *fakeRuntime) { rt.startErr = errors.New("start failed") },
			clear:         func(rt *fakeRuntime) { rt.startErr = nil },
			wantAfterFail: func() []string { return []string{v1(0), v2(0)} },
		},
		{
			name: "recreate: stop of old fails, force remove still clears it", strategy: "recreate", replicas: 1,
			inject:        func(rt *fakeRuntime) { rt.stopErr = errors.New("stop timeout") },
			clear:         func(rt *fakeRuntime) { rt.stopErr = nil },
			wantAfterFail: func() []string { return nil },
		},
		{
			name: "recreate: remove of old fails after stop", strategy: "recreate", replicas: 1,
			inject:        func(rt *fakeRuntime) { rt.removeErr = errors.New("busy") },
			clear:         func(rt *fakeRuntime) { rt.removeErr = nil },
			wantAfterFail: func() []string { return []string{v1(0)} },
		},
		{
			name: "recreate: old removed, create ok, start fails", strategy: "recreate", replicas: 1,
			inject:        func(rt *fakeRuntime) { rt.startErr = errors.New("start failed") },
			clear:         func(rt *fakeRuntime) { rt.startErr = nil },
			wantAfterFail: func() []string { return []string{v2(0)} },
		},
		{
			name: "rolling: second replica create fails, first already replaced", strategy: "rolling", replicas: 2,
			inject: func(rt *fakeRuntime) {
				rt.createErrOnCall = errors.New("pull denied")
				rt.createErrCallNo = 2
			},
			clear:         func(rt *fakeRuntime) { rt.createErrCallNo = 0 },
			wantAfterFail: func() []string { return []string{v2(0), v1(1)} },
		},
		{
			name: "rolling: first replica start fails, no old replica removed", strategy: "rolling", replicas: 2,
			inject:        func(rt *fakeRuntime) { rt.startErr = errors.New("start failed") },
			clear:         func(rt *fakeRuntime) { rt.startErr = nil },
			wantAfterFail: func() []string { return []string{v1(0), v1(1), v2(0)} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			for i := 0; i < tt.replicas; i++ {
				rt.seed(v1(i), true)
			}
			desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: tt.strategy, Replicas: tt.replicas}
			c := New("web", &fakeStore{svc: desired}, rt)

			tt.inject(rt)
			res, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("first pass error = nil, want the injected failure")
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse {
				t.Fatalf("first pass condition = %+v, want False", cond)
			}
			assertNames(t, "after failed pass", rt.names(), tt.wantAfterFail())

			tt.clear(rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("retry pass error = %v, want convergence", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("retry condition = %+v, want True", cond)
			}
			var want []string
			for i := 0; i < tt.replicas; i++ {
				want = append(want, v2(i))
			}
			assertNames(t, "after retry", rt.names(), want)
			for _, name := range want {
				if cs := rt.containers[name]; cs == nil || !cs.Running {
					t.Errorf("container %q running = false after retry, want true", name)
				}
			}
		})
	}
}

func assertNames(t *testing.T, when string, got, want []string) {
	t.Helper()
	set := map[string]bool{}
	for _, n := range got {
		set[n] = true
	}
	if len(got) != len(want) {
		t.Fatalf("containers %s = %v, want %v", when, got, want)
	}
	for _, n := range want {
		if !set[n] {
			t.Fatalf("containers %s = %v, want %v", when, got, want)
		}
	}
}
