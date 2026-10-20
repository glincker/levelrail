package cloudflaretunnel

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestReconcile_HalfSuccess_RecoversOnRetry injects a failure into one step
// of a multi-step change, then clears it and checks the next pass converges
// without duplicating work.
func TestReconcile_HalfSuccess_RecoversOnRetry(t *testing.T) {
	tests := []struct {
		name        string
		seedImage   string // empty: no existing container
		seedRunning bool
		disabled    bool
		inject      func(rt *fakeRuntime)
		clear       func(rt *fakeRuntime)
		wantReason  string
		wantCount   int // containers left after the failed pass
		wantRunning bool
	}{
		{
			name:       "replace: create fails after old removed",
			seedImage:  "cloudflare/cloudflared:old",
			inject:     func(rt *fakeRuntime) { rt.createErr = errors.New("pull denied") },
			clear:      func(rt *fakeRuntime) { rt.createErr = nil },
			wantReason: "ReplaceFailed",
			wantCount:  0,
		},
		{
			name:        "replace: stop fails, old container left untouched",
			seedImage:   "cloudflare/cloudflared:old",
			seedRunning: true,
			inject:      func(rt *fakeRuntime) { rt.stopErr = errors.New("stop timeout") },
			clear:       func(rt *fakeRuntime) { rt.stopErr = nil },
			wantReason:  "ReplaceFailed",
			wantCount:   1,
			wantRunning: true,
		},
		{
			name:       "replace: remove fails after stop",
			seedImage:  "cloudflare/cloudflared:old",
			inject:     func(rt *fakeRuntime) { rt.removeErr = errors.New("busy") },
			clear:      func(rt *fakeRuntime) { rt.removeErr = nil },
			wantReason: "ReplaceFailed",
			wantCount:  1,
		},
		{
			name:       "restart of stopped container fails",
			seedImage:  image,
			inject:     func(rt *fakeRuntime) { rt.startErr = errors.New("cannot start") },
			clear:      func(rt *fakeRuntime) { rt.startErr = nil },
			wantReason: "StartFailed",
			wantCount:  1,
		},
		{
			name:        "disabled: remove fails, container stays and is removed on retry",
			seedImage:   image,
			seedRunning: true,
			disabled:    true,
			inject:      func(rt *fakeRuntime) { rt.removeErr = errors.New("busy") },
			clear:       func(rt *fakeRuntime) { rt.removeErr = nil },
			wantReason:  "RemoveFailed",
			wantCount:   1,
			wantRunning: false, // stop succeeded before remove failed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime()
			if tt.seedImage != "" {
				rt.seed(tt.seedImage, tt.seedRunning)
			}
			c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: !tt.disabled}},
				&fakeTokens{value: "tok", set: true}, rt, WithContainerPrefix("acme"))

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
			if tt.wantCount == 1 {
				st, _ := rt.InspectByName(context.Background(), "acme-cloudflared")
				if st.Running != tt.wantRunning {
					t.Fatalf("Running after failed pass = %v, want %v", st.Running, tt.wantRunning)
				}
			}

			tt.clear(rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("retry pass error = %v, want convergence", err)
			}
			cond := conditionOf(t, res)
			if tt.disabled {
				if rt.count() != 0 || cond.Reason != "Disabled" {
					t.Fatalf("retry: count=%d cond=%+v, want removed and Disabled", rt.count(), cond)
				}
				return
			}
			st, _ := rt.InspectByName(context.Background(), "acme-cloudflared")
			if st == nil || !st.Running || st.Image != image || cond.Status != reconcile.ConditionTrue {
				t.Fatalf("retry: state=%+v cond=%+v, want running current image, Ready", st, cond)
			}
		})
	}
}

// TestReconcile_StartAfterCreateFails_RetryDoesNotDuplicate checks the
// created-but-not-started container is reused by the next pass.
func TestReconcile_StartAfterCreateFails_RetryDoesNotDuplicate(t *testing.T) {
	rt := newFakeRuntime()
	rt.startErr = errors.New("start failed")
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: true}},
		&fakeTokens{value: "tok", set: true}, rt, WithContainerPrefix("acme"))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("first pass error = nil")
	}
	rt.startErr = nil
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1 (retry must reuse the created container)", rt.createCalls)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Errorf("retry condition = %+v, want True", cond)
	}
}
