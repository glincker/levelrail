package database

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_HalfSuccess_RecoversOnRetry injects a failure into
// one step of a multi-step change, then clears it and checks the next pass
// converges to the desired image without a second container appearing.
func TestController_Reconcile_HalfSuccess_RecoversOnRetry(t *testing.T) {
	tests := []struct {
		name        string
		seedImage   string // empty: no existing container
		seedRunning bool
		inject      func(rt *fakeRuntime)
		clear       func(rt *fakeRuntime)
		wantReason  string
		wantCount   int
		wantRunning bool
	}{
		{
			name:       "fresh: volume ensured, create fails",
			inject:     func(rt *fakeRuntime) { rt.createErr = errors.New("pull denied") },
			clear:      func(rt *fakeRuntime) { rt.createErr = nil },
			wantReason: "CreateFailed",
			wantCount:  0,
		},
		{
			name:       "fresh: create succeeds, start fails",
			inject:     func(rt *fakeRuntime) { rt.startErr = errors.New("start failed") },
			clear:      func(rt *fakeRuntime) { rt.startErr = nil },
			wantReason: "StartFailedAfterCreate",
			wantCount:  1,
		},
		{
			name:        "version bump: stop fails, old container untouched",
			seedImage:   "redis:6",
			seedRunning: true,
			inject:      func(rt *fakeRuntime) { rt.stopErr = errors.New("stop timeout") },
			clear:       func(rt *fakeRuntime) { rt.stopErr = nil },
			wantReason:  "ReplaceFailed",
			wantCount:   1,
			wantRunning: true,
		},
		{
			name:        "version bump: remove fails after stop",
			seedImage:   "redis:6",
			seedRunning: true,
			inject:      func(rt *fakeRuntime) { rt.removeErr = errors.New("busy") },
			clear:       func(rt *fakeRuntime) { rt.removeErr = nil },
			wantReason:  "ReplaceFailed",
			wantCount:   1,
		},
		{
			name:        "version bump: old removed, create of new fails",
			seedImage:   "redis:6",
			seedRunning: true,
			inject:      func(rt *fakeRuntime) { rt.createErr = errors.New("pull denied") },
			clear:       func(rt *fakeRuntime) { rt.createErr = nil },
			wantReason:  "ReplaceFailed",
			wantCount:   0,
		},
		{
			name:        "version bump: old removed, new created but start fails",
			seedImage:   "redis:6",
			seedRunning: true,
			inject:      func(rt *fakeRuntime) { rt.startErr = errors.New("start failed") },
			clear:       func(rt *fakeRuntime) { rt.startErr = nil },
			wantReason:  "ReplaceFailed",
			wantCount:   1,
		},
		{
			name:       "restart of crashed container fails",
			seedImage:  "redis:7",
			inject:     func(rt *fakeRuntime) { rt.startErr = errors.New("cannot start") },
			clear:      func(rt *fakeRuntime) { rt.startErr = nil },
			wantReason: "StartFailed",
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime()
			if tt.seedImage != "" {
				rt.seed(containerName("main"), tt.seedImage, tt.seedRunning)
			}
			desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
			c := New("main", &fakeStore{db: desired}, rt)

			tt.inject(rt)
			res, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("first pass error = nil, want the injected failure")
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason != tt.wantReason {
				t.Fatalf("first pass condition = %+v, want False/%s", cond, tt.wantReason)
			}
			if got := rt.count(); got != tt.wantCount {
				t.Fatalf("containers after failed pass = %d, want %d", got, tt.wantCount)
			}
			if tt.name == "version bump: stop fails, old container untouched" {
				st, _ := rt.InspectByName(context.Background(), containerName("main"))
				if st.Running != tt.wantRunning || st.Image != "redis:6" {
					t.Fatalf("old container = %+v, want untouched running redis:6", st)
				}
			}

			tt.clear(rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("retry pass error = %v, want convergence", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("retry condition = %+v, want True", cond)
			}
			if got := rt.count(); got != 1 {
				t.Fatalf("containers after retry = %d, want exactly 1", got)
			}
			st, _ := rt.InspectByName(context.Background(), containerName("main"))
			if st == nil || !st.Running || st.Image != "redis:7" {
				t.Fatalf("retry state = %+v, want running redis:7", st)
			}
		})
	}
}
