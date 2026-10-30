package database

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestVersionBelow(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"v0.8.4", true},
		{"v0.8.5-qa-03f7051d3", true},
		{"v0.8.5", false},
		{"0.8.6", false},
		{"v0.9.0-rc1", false},
		{"v1.0.0", false},
		{"", true},
		{"dev", true},
	}
	for _, tc := range cases {
		if got := versionBelow(tc.v, "0.8.5"); got != tc.want {
			t.Errorf("versionBelow(%q) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

func oldAgent(version string, remote bool) Option {
	return WithAgentVersion(func(context.Context, string) (string, bool, error) { return version, remote, nil })
}

func tlsRedisFixture(t *testing.T, opts ...Option) (*Controller, *fakeRuntime) {
	t.Helper()
	rt := newFakeRuntime()
	rt.seed(containerName("cache"), "redis:7", true)
	// Labels present but without the TLS marker: reads as drift on a current agent.
	rt.containers[containerName("cache")].Labels = map[string]string{"other": "x"}
	certPEM, keyPEM, err := GenerateSelfSignedCert("db-cache")
	if err != nil {
		t.Fatal(err)
	}
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7", NodeID: "node-b"}
	opts = append(opts, WithTLS(&TLSMaterial{CertPEM: certPEM, KeyPEM: keyPEM}))
	return New("cache", &fakeStore{db: desired}, rt, opts...), rt
}

func TestOutdatedAgent_DriftNeverReplaces(t *testing.T) {
	c, rt := tlsRedisFixture(t, oldAgent("v0.8.5-qa-03f7051d3", true))
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != 0 || rt.removeCalls != 0 || rt.stopCalls != 0 {
		t.Fatalf("create=%d remove=%d stop=%d, want none", rt.createCalls, rt.removeCalls, rt.stopCalls)
	}
	var note *reconcile.Condition
	for i := range res.Conditions {
		if res.Conditions[i].Type == ConditionTypeAgentCompatible {
			note = &res.Conditions[i]
		}
	}
	if note == nil || note.Reason != reasonAgentOutdated || note.Status != reconcile.ConditionFalse {
		t.Fatalf("want AgentOutdated condition, got %+v", res.Conditions)
	}
}

func TestOutdatedAgent_PortDriftNeverReplaces(t *testing.T) {
	rt := newFakeRuntime()
	rt.seedWithPorts(containerName("cache"), "redis:7", true, []docker.PortBinding{{ContainerPort: 6379, HostPort: 1111, HostIP: "0.0.0.0"}})
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7", NodeID: "node-b"}
	c := New("cache", &fakeStore{db: desired}, rt, oldAgent("v0.8.4", true))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.removeCalls != 0 {
		t.Fatalf("remove=%d, want 0 on an outdated agent", rt.removeCalls)
	}
}

func TestOutdatedAgent_ExplicitImageMismatchStillActs(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("cache"), "redis:6", true)
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7", NodeID: "node-b"}
	c := New("cache", &fakeStore{db: desired}, rt, oldAgent("v0.8.4", true))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("create=%d, want 1 for an explicit image change", rt.createCalls)
	}
}

func TestCurrentAndLocalAgents_StillReplaceOnDrift(t *testing.T) {
	for name, opt := range map[string]Option{
		"current": oldAgent("v0.8.5", true),
		"local":   oldAgent("", false),
	} {
		c, rt := tlsRedisFixture(t, opt)
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if rt.stopCalls != 1 {
			t.Errorf("%s: stop=%d, want 1 (drift acts on a trusted agent)", name, rt.stopCalls)
		}
	}
}

func TestOutdatedAgent_LookupErrorIsTreatedAsOutdated(t *testing.T) {
	c, rt := tlsRedisFixture(t, WithAgentVersion(func(context.Context, string) (string, bool, error) {
		return "", true, errors.New("node row unavailable")
	}))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.removeCalls != 0 {
		t.Fatalf("remove=%d, want 0", rt.removeCalls)
	}
}

// A replace must reuse the named data volume and never leave a mount that
// would let the image's VOLUME directive create an anonymous one.
func TestReplace_KeepsNamedDataVolume(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("cache"), "redis:6", true)
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}
	c := New("cache", &fakeStore{db: desired}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	spec := rt.lastCreateSpec
	var named bool
	for _, v := range spec.Volumes {
		if v.Name == "" {
			t.Fatalf("anonymous mount at %s", v.ContainerPath)
		}
		if v.Name == dataVolumeName("cache") {
			named = true
		}
	}
	if !named {
		t.Fatalf("data volume %q not mounted: %+v", dataVolumeName("cache"), spec.Volumes)
	}
}

func TestReplace_MissingDataVolumeFailsLoudly(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("cache"), "redis:6", true)
	vr := &volumeRuntime{fakeRuntime: rt}
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}
	c := New("cache", &fakeStore{db: desired}, vr)
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	if got := conditionOf(t, res).Reason; got != reasonDataVolumeMissing {
		t.Fatalf("reason = %q, want %q", got, reasonDataVolumeMissing)
	}
	if rt.removeCalls != 0 || rt.stopCalls != 0 || rt.createCalls != 0 {
		t.Fatalf("destructive calls ran: remove=%d stop=%d create=%d", rt.removeCalls, rt.stopCalls, rt.createCalls)
	}
}

func TestCheckNamedMounts_RejectsAnonymous(t *testing.T) {
	spec := docker.ContainerSpec{Name: "db-x", Volumes: []docker.VolumeMount{{ContainerPath: "/data"}}}
	if checkNamedMounts(spec, "db-x-data") == nil {
		t.Fatal("want an error for a mount without a volume name")
	}
	spec.Volumes = []docker.VolumeMount{{Name: "other", ContainerPath: "/data"}}
	if checkNamedMounts(spec, "db-x-data") == nil {
		t.Fatal("want an error when the data volume is not mounted")
	}
}
