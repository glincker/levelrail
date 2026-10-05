package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_HalfSuccess_FirstDeployAndRestartRecoverOnRetry
// injects one failure at each step before or after Create on a fresh deploy
// and on a crashed-container restart, then checks the failed pass leaves no
// duplicate or orphan and the next pass converges to one running container.
func TestController_Reconcile_HalfSuccess_FirstDeployAndRestartRecoverOnRetry(t *testing.T) {
	name := replicaContainerName("web", "img:v1", "", 0)
	dbName := database.ContainerName("main")

	tests := []struct {
		name      string
		seedStop  bool
		withDB    bool
		inject    func(rt *fakeRuntime)
		clear     func(rt *fakeRuntime)
		wantAfter []string
	}{
		{
			name:      "fresh: volume ensure fails, nothing created",
			inject:    func(rt *fakeRuntime) { rt.ensureVolumeErr = errors.New("volume driver unavailable") },
			clear:     func(rt *fakeRuntime) { rt.ensureVolumeErr = nil },
			wantAfter: nil,
		},
		{
			name:      "fresh: network ensure fails, nothing created",
			inject:    func(rt *fakeRuntime) { rt.ensureNetworkErr = errors.New("daemon busy") },
			clear:     func(rt *fakeRuntime) { rt.ensureNetworkErr = nil },
			wantAfter: nil,
		},
		{
			name:      "fresh: network ready, database attach fails, app not created",
			withDB:    true,
			inject:    func(rt *fakeRuntime) { rt.networkConnectErr = errors.New("endpoint busy") },
			clear:     func(rt *fakeRuntime) { rt.networkConnectErr = nil },
			wantAfter: []string{dbName},
		},
		{
			name:      "fresh: create ok, start fails, container removed so retry cannot duplicate",
			inject:    func(rt *fakeRuntime) { rt.startErr = errors.New("port is already allocated") },
			clear:     func(rt *fakeRuntime) { rt.startErr = nil },
			wantAfter: nil,
		},
		{
			name: "fresh: create ok, start fails, cleanup remove fails, stopped orphan restarted on retry",
			inject: func(rt *fakeRuntime) {
				rt.startErr = errors.New("start failed")
				rt.removeErr = errors.New("busy")
			},
			clear:     func(rt *fakeRuntime) { rt.startErr = nil; rt.removeErr = nil },
			wantAfter: []string{name},
		},
		{
			name:      "crashed: network ensure fails before restart, container untouched",
			seedStop:  true,
			inject:    func(rt *fakeRuntime) { rt.ensureNetworkErr = errors.New("daemon busy") },
			clear:     func(rt *fakeRuntime) { rt.ensureNetworkErr = nil },
			wantAfter: []string{name},
		},
		{
			name:     "crashed: restart fails and broken container cannot be removed",
			seedStop: true,
			inject: func(rt *fakeRuntime) {
				rt.startErr = errors.New("start failed")
				rt.removeErr = errors.New("busy")
			},
			clear:     func(rt *fakeRuntime) { rt.startErr = nil; rt.removeErr = nil },
			wantAfter: []string{name},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			var opts []Option
			if tt.seedStop {
				rt.seed(name, false)
			}
			desired := &store.DesiredService{
				Name: "web", AppID: "app1", Image: "img:v1", Port: 80,
				Volumes: []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/data"}},
			}
			if tt.withDB {
				rt.seed(dbName, true)
				desired.DatabaseEnv = map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main", Field: "url"}}
				opts = append(opts,
					WithDatabaseAttachments(&fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
						"main": {Name: "main", Engine: store.EnginePostgres},
					}}),
					WithSecretResolver(newFakeSecretResolver(map[string]string{"main/" + database.PostgresPasswordEnvKey: "pw"})))
			}
			c := New("web", &fakeStore{svc: desired}, rt, opts...)

			tt.inject(rt)
			res, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("first pass error = nil, want the injected failure")
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse {
				t.Fatalf("first pass condition = %+v, want False", cond)
			}
			assertNames(t, "after failed pass", rt.names(), tt.wantAfter)

			tt.clear(rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("retry pass error = %v, want convergence", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("retry condition = %+v, want True", cond)
			}
			want := []string{name}
			if tt.withDB {
				want = append(want, dbName)
			}
			assertNames(t, "after retry", rt.names(), want)
			if cs := rt.containers[name]; cs == nil || !cs.Running {
				t.Errorf("container %q running = false after retry, want true", name)
			}
			if tt.withDB && len(rt.connections) != 1 {
				t.Errorf("database network connections = %v, want exactly one after retry", rt.connections)
			}
		})
	}
}
