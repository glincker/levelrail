package ingress

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestFailoverRoute(t *testing.T) {
	newSvc := store.DesiredService{Name: "web", Image: "web:3", Port: 80}
	oldSvc := store.DesiredService{Name: "web", Image: "web:2", Port: 80}
	newName := application.ContainerName(newSvc.Name, application.NameImage(newSvc), newSvc.RestartNonce)
	oldName := application.ContainerName(oldSvc.Name, application.NameImage(oldSvc), oldSvc.RestartNonce)

	tests := []struct {
		name      string
		env       *string
		seedOld   bool
		oldStops  bool
		dial      string
		wantNil   bool
		wantUps   []string
		wantTryOn string
	}{
		{name: "held previous becomes standby", seedOld: true, dial: "127.0.0.1:1111", wantUps: []string{"127.0.0.1:1111", "127.0.0.1:2222"}, wantTryOn: "5s"},
		{name: "no previous release", dial: "127.0.0.1:1111", wantNil: true},
		{name: "previous not running", seedOld: true, oldStops: true, dial: "127.0.0.1:1111", wantNil: true},
		{name: "already routed to previous", seedOld: true, dial: "127.0.0.1:2222", wantNil: true},
		{name: "env override", env: ptr("2s"), seedOld: true, dial: "127.0.0.1:1111", wantUps: []string{"127.0.0.1:1111", "127.0.0.1:2222"}, wantTryOn: "2s"},
		{name: "env off", env: ptr("off"), seedOld: true, dial: "127.0.0.1:1111", wantNil: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != nil {
				t.Setenv(failoverEnv, *tc.env)
			}
			rt := newFakeRuntime()
			rt.seedRunning(newName, 1111)
			if tc.seedOld {
				if tc.oldStops {
					rt.seedStopped(oldName)
				} else {
					rt.seedRunning(oldName, 2222)
				}
			}
			rt.containers[newName].Image = newSvc.Image
			if tc.seedOld {
				rt.containers[oldName].Image = oldSvc.Image
			}
			c := New(&fakeStore{}, rt, &fakeApplier{}, WithLogger(discardLogger()))
			lb := c.failoverRoute(context.Background(), newSvc, tc.dial)
			if tc.wantNil {
				if lb != nil {
					t.Fatalf("lb = %+v, want nil", lb)
				}
				return
			}
			if lb == nil {
				t.Fatal("lb = nil, want a failover pool")
			}
			if len(lb.Upstreams) != 2 || lb.Upstreams[0] != tc.wantUps[0] || lb.Upstreams[1] != tc.wantUps[1] {
				t.Fatalf("upstreams = %v, want %v", lb.Upstreams, tc.wantUps)
			}
			if lb.Policy != "first" || lb.TryDuration != tc.wantTryOn || lb.PassiveHealth == nil || lb.PassiveHealth.MaxFails != 1 {
				t.Fatalf("lb = %+v", lb)
			}
		})
	}
}

func ptr(s string) *string { return &s }
