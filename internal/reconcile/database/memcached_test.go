package database

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestController_Reconcile_Memcached(t *testing.T) {
	const mib = 1 << 20
	tests := []struct {
		name      string
		resources *store.ServiceResources
		wantMB    string
	}{
		{"no limit uses the memcached default", nil, "64"},
		{"three quarters of the limit", &store.ServiceResources{MemoryBytes: 512 * mib}, "384"},
		{"tiny limit is floored", &store.ServiceResources{MemoryBytes: 8 * mib}, "16"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := newFakeRuntime()
			desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineMemcached, Version: "1.6", Resources: tc.resources}
			res, err := New("cache", &fakeStore{db: desired}, rt).Reconcile(context.Background())
			if err != nil || conditionOf(t, res).Status != reconcile.ConditionTrue {
				t.Fatalf("Reconcile() = %+v, %v", res, err)
			}
			spec := rt.lastCreateSpec
			want := []string{"memcached", "-m", tc.wantMB, "-U", "0"}
			if spec.Image != "memcached:1.6" || len(spec.Command) != len(want) {
				t.Fatalf("image = %q command = %q", spec.Image, spec.Command)
			}
			for i := range want {
				if spec.Command[i] != want[i] {
					t.Fatalf("command = %q, want %q", spec.Command, want)
				}
			}
			if len(spec.Volumes) != 0 || rt.ensureVolumeCalls != 0 || len(spec.Env) != 0 {
				t.Fatalf("memcached must get no volume and no env: volumes %+v, ensure calls %d, env %v", spec.Volumes, rt.ensureVolumeCalls, spec.Env)
			}
		})
	}
}

func TestController_Reconcile_Memcached_MemoryChangeReplaces(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineMemcached, Version: "1.6"}
	c := New("cache", &fakeStore{db: desired}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt.containers[containerName("cache")].Labels = rt.lastCreateSpec.Labels
	desired.Resources = &store.ServiceResources{MemoryBytes: 256 << 20}
	if _, err := c.Reconcile(context.Background()); err != nil || rt.createCalls != 2 {
		t.Fatalf("a new cache size must replace the container: err = %v creates = %d", err, rt.createCalls)
	}
}

func TestController_Reconcile_Memcached_RefusesNonLoopbackExposure(t *testing.T) {
	for _, bind := range []string{bindaddr.Public, "10.0.0.5"} {
		rt := newFakeRuntime()
		desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineMemcached, Version: "1.6", PubliclyAccessible: true, PublicPort: 21211, PublicBindAddress: bind}
		res, err := New("cache", &fakeStore{db: desired}, rt).Reconcile(context.Background())
		if err != nil || conditionOf(t, res).Reason != "UnauthenticatedExposureRefused" || rt.createCalls != 0 {
			t.Fatalf("bind %q: res = %+v err = %v creates = %d", bind, res, err, rt.createCalls)
		}
	}
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineMemcached, Version: "1.6", PubliclyAccessible: true, PublicPort: 21211, PublicBindAddress: bindaddr.Private}
	if _, err := New("cache", &fakeStore{db: desired}, rt).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p := rt.lastCreateSpec.Ports; len(p) != 1 || p[0].HostIP != "127.0.0.1" || p[0].ContainerPort != 11211 {
		t.Fatalf("ports = %+v, want loopback only", p)
	}
}

func TestMemcachedAddressingAndVersions(t *testing.T) {
	if p, ok := ContainerPort(store.EngineMemcached); !ok || p != 11211 {
		t.Fatalf("ContainerPort = %d, %v", p, ok)
	}
	for field, want := range map[string]bool{"url": true, "host": true, "port": true, "password": false, "username": false, "database": false} {
		if got := SupportsField(store.EngineMemcached, field); got != want {
			t.Errorf("SupportsField(memcached, %s) = %v, want %v", field, got, want)
		}
	}
	if got := ConventionalEnvVar(store.EngineMemcached, "url"); got != "MEMCACHED_SERVERS" {
		t.Errorf("ConventionalEnvVar(url) = %q", got)
	}
	if got := ConventionalEnvVar(store.EngineMemcached, "password"); got != "" {
		t.Errorf("memcached has no password variable, got %q", got)
	}
	if ValidateEngineVersion(store.EngineMemcached, "1.6") != nil || ValidateEngineVersion(store.EngineMemcached, "latest") == nil {
		t.Error("memcached needs a pinned 1.x tag")
	}
	if err := CheckInPlaceVersionChange(store.EngineMemcached, "1.6.45", "1.6.40"); err != nil {
		t.Errorf("memcached has no data to protect, any pinned change is fine: %v", err)
	}
}

func TestParseMemcachedStats(t *testing.T) {
	stats, err := ParseMemcachedStats("STAT pid 1\r\nSTAT version 1.6.45\r\nSTAT curr_items 12\r\nEND\r\n")
	if err != nil || stats["version"] != "1.6.45" || stats["curr_items"] != "12" {
		t.Fatalf("stats = %v, %v", stats, err)
	}
	if _, err := ParseMemcachedStats("STAT pid 1\r\n"); err == nil {
		t.Fatal("a cut-off answer must be an error")
	}
}
