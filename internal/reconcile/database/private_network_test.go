package database

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// networkRecorder records network calls the shared fake ignores.
type networkRecorder struct {
	*fakeRuntime
	ensured      []string
	connected    []string
	disconnected []string
}

func (r *networkRecorder) EnsureNetwork(_ context.Context, name string) (string, error) {
	r.ensured = append(r.ensured, name)
	return "id-" + name, nil
}

func (r *networkRecorder) NetworkConnect(_ context.Context, name, _ string) error {
	r.connected = append(r.connected, name)
	return nil
}

func (r *networkRecorder) NetworkDisconnect(_ context.Context, name, _ string, _ bool) error {
	r.disconnected = append(r.disconnected, name)
	return nil
}

func TestManagedDatabaseSpec_PrivateNetworkOnlyNoPublishedPort(t *testing.T) {
	cases := []struct {
		engine string
		opts   []Option
	}{
		{engine: store.EngineRedis},
		{engine: store.EngineValkey},
		{engine: store.EnginePostgres, opts: []Option{WithPostgresCredentials(&PostgresCredentials{Username: "u", Password: "p"})}},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			rt := &networkRecorder{fakeRuntime: newFakeRuntime()}
			desired := &store.DesiredDatabase{Name: "main", Engine: tc.engine, Version: "16"}
			if _, err := New("main", &fakeStore{db: desired}, rt, tc.opts...).Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			spec := rt.lastCreateSpec
			want := PrivateNetworkName("main")
			if spec.NetworkMode != want || spec.Network == nil || spec.Network.Name != want || spec.Network.Alias != containerName("main") {
				t.Fatalf("network mode=%q attach=%+v, want only %q with alias %q", spec.NetworkMode, spec.Network, want, containerName("main"))
			}
			if spec.NetworkMode == defaultBridgeNetwork || spec.NetworkMode == "" {
				t.Fatalf("spec would join the default bridge: %q", spec.NetworkMode)
			}
			if len(spec.Ports) != 0 {
				t.Fatalf("ports = %+v, want none by default", spec.Ports)
			}
			if len(rt.ensured) == 0 || rt.ensured[0] != want {
				t.Fatalf("ensured networks = %v, want %q before create", rt.ensured, want)
			}
		})
	}
}

func TestManagedDatabase_ExistingContainerLeavesDefaultBridge(t *testing.T) {
	rt := &networkRecorder{fakeRuntime: newFakeRuntime()}
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	rt.seed(containerName("main"), ImageRef(store.EngineRedis, "7"), true)
	rt.containers[containerName("main")].Networks = []docker.ContainerNetwork{{Name: defaultBridgeNetwork}}

	if _, err := New("main", &fakeStore{db: desired}, rt).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want a live move with no recreate", rt.createCalls)
	}
	if len(rt.connected) != 1 || rt.connected[0] != PrivateNetworkName("main") {
		t.Errorf("connected = %v, want the private network", rt.connected)
	}
	if len(rt.disconnected) != 1 || rt.disconnected[0] != defaultBridgeNetwork {
		t.Errorf("disconnected = %v, want the default bridge", rt.disconnected)
	}
}
