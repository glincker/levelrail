package plugin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func mustPub(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestContainerSpecIsLockedDown(t *testing.T) {
	h := newHarness(t)
	for _, hosts := range []string{"[]", "[hooks.example.com]"} {
		rec := Record{Name: "example", Image: "registry.example.com/team/example@" + testDigest}
		if hosts != "[]" {
			rec.OutboundHosts = []string{"hooks.example.com"}
		}
		spec := h.m.containerSpec(rec)
		if !spec.Hardened {
			t.Error("spec is not hardened")
		}
		if spec.User != "1000:1000" {
			t.Errorf("User = %q, want a non-root uid", spec.User)
		}
		if spec.NetworkMode == "host" {
			t.Error("host network")
		}
		if len(spec.CapAdd) != 0 || len(spec.Volumes) != 0 || len(spec.Ports) != 0 {
			t.Errorf("spec adds capabilities, volumes or ports: %+v", spec)
		}
		if len(spec.BindMounts) != 1 || spec.BindMounts[0].ContainerPath != containerSocketDir {
			t.Fatalf("bind mounts = %+v, want only the socket dir", spec.BindMounts)
		}
		for _, bm := range spec.BindMounts {
			if strings.Contains(bm.HostPath, "docker.sock") || bm.HostPath == "/" {
				t.Errorf("dangerous bind mount %+v", bm)
			}
		}
		if spec.Resources == nil || spec.Resources.MemoryBytes <= 0 || spec.Resources.NanoCPUs <= 0 || spec.PidsLimit <= 0 {
			t.Errorf("missing limits: %+v pids=%d", spec.Resources, spec.PidsLimit)
		}
		if len(spec.Env) != 1 {
			t.Errorf("env = %v, want only the socket path", spec.Env)
		}
		if hosts == "[]" && spec.NetworkMode != "none" {
			t.Errorf("a plugin with no outbound hosts got network mode %q, want none", spec.NetworkMode)
		}
		if hosts != "[]" && (spec.Network == nil || spec.NetworkMode != "") {
			t.Errorf("a plugin with outbound hosts should join its dedicated network: %+v", spec)
		}
	}
}

func TestReconcileStartsEnabledPlugin(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	h.m.Reconcile(ctx)
	if h.dk.count() != 1 || len(h.dk.specs) != 1 {
		t.Fatalf("containers=%d specs=%d, want one running plugin", h.dk.count(), len(h.dk.specs))
	}
	if h.m.inst("example").connected() == nil {
		t.Fatal("no client after a successful start")
	}
	h.m.Reconcile(ctx)
	if len(h.dk.specs) != 1 {
		t.Fatal("a healthy plugin was recreated")
	}
}

func TestStartHalfSucceededThenCleanupRetried(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})

	h.dk.startErr = errBoom
	h.dk.removeErr = errBoom
	h.m.Reconcile(ctx)
	if h.dk.count() != 1 {
		t.Fatalf("expected the created-but-unstarted container to linger, got %d", h.dk.count())
	}
	rec, _ := h.store.GetPlugin(ctx, "example")
	if !strings.Contains(rec.LastError, "start container") {
		t.Fatalf("LastError = %q, want the start failure", rec.LastError)
	}

	h.m.Reconcile(ctx)
	if len(h.dk.specs) != 1 {
		t.Fatal("retried before the backoff elapsed")
	}

	h.dk.startErr, h.dk.removeErr = nil, nil
	h.advance(time.Minute)
	h.m.Reconcile(ctx)
	if len(h.dk.removed) == 0 {
		t.Fatal("the stale container from the failed start was never cleaned up")
	}
	if h.dk.count() != 1 || h.m.inst("example").connected() == nil {
		t.Fatalf("plugin did not recover: containers=%d", h.dk.count())
	}
	rec, _ = h.store.GetPlugin(ctx, "example")
	if rec.LastError != "" {
		t.Fatalf("LastError = %q after recovery", rec.LastError)
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	h.dk.createErr = errBoom
	var gaps []time.Duration
	for range 12 {
		h.m.Reconcile(ctx)
		in := h.m.inst("example")
		in.mu.Lock()
		gap := in.nextAttempt.Sub(h.now)
		in.mu.Unlock()
		gaps = append(gaps, gap)
		h.advance(gap)
	}
	if gaps[0] != 2*time.Second || gaps[1] != 4*time.Second || gaps[2] != 8*time.Second {
		t.Fatalf("backoff = %v, want doubling from 2s", gaps[:3])
	}
	if gaps[len(gaps)-1] != 5*time.Minute {
		t.Fatalf("backoff did not cap at 5m: %v", gaps)
	}
}

func TestHandshakeMismatchFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		edit func(h *harness)
	}{
		{"wrong name", func(h *harness) { h.setDescribe("impostor", []string{AbilityRead}) }},
		{"asks for more than the manifest", func(h *harness) { h.setDescribe("example", []string{AbilityRead, AbilityReadSensitive}) }},
		{"unsupported contract version", func(h *harness) { h.rpc.describe.ContractVersion = 9 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.installApprovedEnabled("[read]", []string{AbilityRead})
			tt.edit(h)
			h.m.Reconcile(context.Background())
			if h.m.inst("example").connected() != nil {
				t.Fatal("plugin accepted despite a bad handshake")
			}
			if h.dk.count() != 0 {
				t.Fatal("container left running after a rejected handshake")
			}
		})
	}
}

func TestRepeatedHealthFailuresRestart(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	h.m.Reconcile(ctx)
	h.rpc.healthOK = false
	for range 3 {
		h.m.Reconcile(ctx)
	}
	if h.m.inst("example").connected() != nil || h.dk.count() != 0 {
		t.Fatal("unhealthy plugin was left running")
	}
	h.rpc.healthOK = true
	h.advance(time.Minute)
	h.m.Reconcile(ctx)
	if h.m.inst("example").connected() == nil {
		t.Fatal("plugin did not come back after recovering")
	}
}

func TestRevokedKeyStopsPlugin(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	h.m.Reconcile(ctx)
	if err := h.m.TrustRemove(ctx, h.keyID, "admin"); err != nil {
		t.Fatal(err)
	}
	h.m.Reconcile(ctx)
	if h.dk.count() != 0 || h.m.inst("example").connected() != nil {
		t.Fatal("a plugin whose signing key was removed kept running")
	}
	rec, _ := h.store.GetPlugin(ctx, "example")
	if !strings.Contains(rec.LastError, "no longer verifies") {
		t.Fatalf("LastError = %q", rec.LastError)
	}
	h.advance(time.Hour)
	h.m.Reconcile(ctx)
	if h.dk.count() != 0 {
		t.Fatal("plugin restarted without a trusted key")
	}
}

func TestTamperedStoredManifestIsNotRun(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	rec := h.store.plugins["example"]
	rec.Image = "evil.example.com/x@sha256:" + strings.Repeat("c", 64)
	h.store.plugins["example"] = rec
	h.m.Reconcile(ctx)
	if h.dk.count() != 0 {
		t.Fatal("started an image that does not match the signed manifest")
	}
}

func TestDisableAndOrphanCleanup(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	h.m.Reconcile(ctx)
	if _, err := h.m.Disable(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	if h.dk.count() != 0 {
		t.Fatal("disable left the container running")
	}
	h.dk.containers["ns-plugin-ghost"] = &docker.ContainerState{ID: "g1", Name: "ns-plugin-ghost", Labels: map[string]string{"ns.plugin": "ghost"}}
	h.dk.containers["ns-plugin-unrelated"] = &docker.ContainerState{ID: "u1", Name: "ns-plugin-unrelated"}
	h.m.Reconcile(ctx)
	if _, ok := h.dk.containers["ns-plugin-ghost"]; ok {
		t.Fatal("orphaned plugin container survived")
	}
	if _, ok := h.dk.containers["ns-plugin-unrelated"]; !ok {
		t.Fatal("removed a container that carries no plugin label")
	}
}

func TestRemoveRetriesWhenContainerCleanupFails(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	h.m.Reconcile(ctx)

	h.dk.removeErr = errBoom
	if err := h.m.Remove(ctx, "example", "admin"); err == nil {
		t.Fatal("Remove hid a cleanup failure")
	}
	rec, err := h.store.GetPlugin(ctx, "example")
	if err != nil || rec.State != StateRemoving {
		t.Fatalf("record after failed cleanup = %+v, %v; want it kept as removing", rec, err)
	}

	h.dk.removeErr = nil
	h.m.Reconcile(ctx)
	if _, err := h.store.GetPlugin(ctx, "example"); err == nil {
		t.Fatal("plugin row survived the retry")
	}
	if h.dk.count() != 0 {
		t.Fatal("container survived the retry")
	}
}
