package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestReconcile_HalfSuccess_RecoversOnRetry injects a failure into one step
// of a multi-step change, then clears it and checks the next pass converges.
func TestReconcile_HalfSuccess_RecoversOnRetry(t *testing.T) {
	tests := []struct {
		name        string
		seedImage   string // empty: no existing container
		seedRunning bool
		disabled    bool
		inject      func(rt *fakeRuntime)
		clear       func(rt *fakeRuntime)
		wantReason  string
		wantCount   int
		wantRunning bool
		wantVolume  bool
	}{
		{
			name:       "fresh: create succeeds, start fails",
			inject:     func(rt *fakeRuntime) { rt.startErr = errors.New("start failed") },
			clear:      func(rt *fakeRuntime) { rt.startErr = nil },
			wantReason: "CreateFailed",
			wantCount:  1,
			wantVolume: true,
		},
		{
			name:       "fresh: volume created, container create fails",
			inject:     func(rt *fakeRuntime) { rt.createErr = errors.New("pull denied") },
			clear:      func(rt *fakeRuntime) { rt.createErr = nil },
			wantReason: "CreateFailed",
			wantCount:  0,
			wantVolume: true,
		},
		{
			name:       "fresh: volume ensure fails, nothing created",
			inject:     func(rt *fakeRuntime) { rt.ensureVolumeErr = errors.New("disk full") },
			clear:      func(rt *fakeRuntime) { rt.ensureVolumeErr = nil },
			wantReason: "VolumeFailed",
			wantCount:  0,
		},
		{
			name:       "replace: create fails after old removed",
			seedImage:  "registry:1",
			inject:     func(rt *fakeRuntime) { rt.createErr = errors.New("pull denied") },
			clear:      func(rt *fakeRuntime) { rt.createErr = nil },
			wantReason: "ReplaceFailed",
			wantCount:  0,
			wantVolume: true,
		},
		{
			name:        "replace: stop fails, old container untouched",
			seedImage:   "registry:1",
			seedRunning: true,
			inject:      func(rt *fakeRuntime) { rt.stopErr = errors.New("stop timeout") },
			clear:       func(rt *fakeRuntime) { rt.stopErr = nil },
			wantReason:  "ReplaceFailed",
			wantCount:   1,
			wantRunning: true,
			wantVolume:  true,
		},
		{
			name:       "restart of stopped container fails",
			seedImage:  image,
			inject:     func(rt *fakeRuntime) { rt.startErr = errors.New("cannot start") },
			clear:      func(rt *fakeRuntime) { rt.startErr = nil },
			wantReason: "StartFailed",
			wantCount:  1,
			wantVolume: true,
		},
		{
			name:        "disabled: remove fails, removed on retry",
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
			settings := store.RegistrySettings{Enabled: !tt.disabled, Host: "registry.example", Username: "levelrail"}
			c := New(&fakeStore{settings: settings}, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"))

			tt.inject(rt)
			res, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("first pass error = nil, want the injected failure")
			}
			assertCondition(t, res, reconcile.ConditionFalse, tt.wantReason)
			if got := rt.count(); got != tt.wantCount {
				t.Fatalf("containers after failed pass = %d, want %d", got, tt.wantCount)
			}
			if tt.wantCount == 1 {
				st, _ := rt.InspectByName(context.Background(), "acme-registry")
				if st.Running != tt.wantRunning {
					t.Fatalf("Running after failed pass = %v, want %v", st.Running, tt.wantRunning)
				}
			}
			if got := rt.hasVolume(VolumeName("acme")); got != tt.wantVolume {
				t.Fatalf("volume present = %v, want %v", got, tt.wantVolume)
			}

			tt.clear(rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("retry pass error = %v, want convergence", err)
			}
			if tt.disabled {
				if rt.count() != 0 {
					t.Fatalf("retry: containers = %d, want 0", rt.count())
				}
				assertCondition(t, res, reconcile.ConditionUnknown, "Disabled")
				return
			}
			st, _ := rt.InspectByName(context.Background(), "acme-registry")
			if st == nil || !st.Running || st.Image != image {
				t.Fatalf("retry: state = %+v, want running current image", st)
			}
			assertCondition(t, res, reconcile.ConditionTrue, "Running")
		})
	}
}

// TestReconcile_StartAfterCreateFails_RetryDoesNotDuplicate checks the
// created-but-not-started container is reused by the next pass.
func TestReconcile_StartAfterCreateFails_RetryDoesNotDuplicate(t *testing.T) {
	rt := newFakeRuntime()
	rt.startErr = errors.New("start failed")
	settings := store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}
	c := New(&fakeStore{settings: settings}, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("first pass error = nil")
	}
	rt.startErr = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}
}
