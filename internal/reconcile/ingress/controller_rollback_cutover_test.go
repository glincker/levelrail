package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startedRuntime reports each container's process start, which a restart in
// place moves while Created stays put.
type startedRuntime struct {
	*fakeRuntime
	started map[string]time.Time
}

func (r *startedRuntime) InspectExitState(ctx context.Context, name string) (*docker.ExitState, error) {
	cs, err := r.InspectByName(ctx, name)
	if err != nil || cs == nil {
		return nil, err
	}
	return &docker.ExitState{Running: cs.Running, StartedAt: r.started[name]}, nil
}

func attemptAt(image string, at time.Time) store.DeployAttempt {
	return store.DeployAttempt{Image: image, Status: store.DeployAttemptStatusSucceeded, StartedAt: at}
}

// rollbackRig is v2 serving, v1 held, and a rollback to v1 that started at
// rollbackAt. The v1 process started at v1Started.
func rollbackRig(t *testing.T, v1Started time.Time, notReady bool, readyAt time.Time) (*Controller, *fakeApplier) {
	t.Helper()
	now := time.Now()
	base := now.Add(-time.Hour)
	rollbackAt := now.Add(-5 * time.Second)
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	fake := newFakeRuntime()
	seedRelease(fake, "web", "img:v1", 34561, base)
	seedRelease(fake, "web", "img:v2", 34562, base.Add(3*time.Minute))
	rt := &startedRuntime{fakeRuntime: fake, started: map[string]time.Time{
		application.ContainerName("web", "img:v1", ""): v1Started,
		application.ContainerName("web", "img:v2", ""): base.Add(3 * time.Minute),
	}}
	st := &fakeStore{
		services: []store.DesiredService{desired},
		attempts: map[string][]store.DeployAttempt{"web": {
			attemptAt("img:v1", rollbackAt), attemptAt("img:v2", base.Add(3*time.Minute)), attemptAt("img:v1", base),
		}},
		readyAt: map[string]time.Time{"web": readyAt},
	}
	if notReady {
		st.notReadyServices = map[string]bool{"web": true}
	}
	applier := &fakeApplier{}
	return New(st, rt, applier, WithLogger(discardLogger())), applier
}

// R1-1: a digest-pinned rollback restarts the held v1 container in place.
// Until the restarted process passes readiness, traffic stays on v2.
func TestRollback_RestartInPlace_WaitsForReadiness(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		started  time.Time
		notReady bool
		readyAt  time.Time
		want     string
	}{
		{"pending readiness after restart", now, true, time.Time{}, "127.0.0.1:34562"},
		{"stale Ready from before the rollback", now, false, now.Add(-10 * time.Second), "127.0.0.1:34562"},
		{"Ready written after the rollback", now.Add(-4 * time.Second), false, now, "127.0.0.1:34561"},
		{"held release never stopped", now.Add(-time.Hour), true, time.Time{}, "127.0.0.1:34561"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, applier := rollbackRig(t, tt.started, tt.notReady, tt.readyAt)
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := onlyUpstream(t, applier); got != tt.want {
				t.Errorf("dial = %s, want %s", got, tt.want)
			}
		})
	}
}

// F1-4: a rollback by digest gets a new container name. While it warms up,
// the held v1 container runs the same content and serves instead of v2.
func TestRollback_DifferentRef_PrefersHeldSameContent(t *testing.T) {
	now := time.Now()
	base := now.Add(-time.Hour)
	desired := store.DesiredService{
		Name: "web", Image: "img@sha256:aaa", ImageID: "sha256:idA", ImageIDRef: "img@sha256:aaa",
		Port: 80, Domains: []string{"web.example.com"},
	}
	rt := newFakeRuntime()
	target := application.ContainerName("web", application.NameImage(desired), "")
	rt.seedRunning(target, 34563)
	rt.containers[target].Image, rt.containers[target].ImageID, rt.containers[target].Created = desired.Image, "sha256:idA", now
	seedRelease(rt, "web", "img:v1", 34561, base)
	rt.containers[application.ContainerName("web", "img:v1", "")].ImageID = "sha256:idA"
	seedRelease(rt, "web", "img:v2", 34562, base.Add(3*time.Minute))
	rt.containers[application.ContainerName("web", "img:v2", "")].ImageID = "sha256:idB"
	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
		attempts: map[string][]store.DeployAttempt{"web": {
			attemptAt(desired.Image, now.Add(-5*time.Second)), attemptAt("img:v2", base.Add(3*time.Minute)), attemptAt("img:v1", base),
		}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34561" {
		t.Errorf("dial = %s, want the held v1 container 127.0.0.1:34561", got)
	}
}

// A forward deploy keeps traffic on the serving release while the new one
// warms up, whatever image IDs the older releases carry.
func TestDeploy_NewContentWarming_StaysOnServingRelease(t *testing.T) {
	now := time.Now()
	base := now.Add(-time.Hour)
	desired := store.DesiredService{Name: "web", Image: "img:v3", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	seedRelease(rt, "web", "img:v3", 34563, now)
	seedRelease(rt, "web", "img:v2", 34562, base.Add(3*time.Minute))
	seedRelease(rt, "web", "img:v1", 34561, base)
	for tag, id := range map[string]string{"v3": "sha256:idC", "v2": "sha256:idB", "v1": "sha256:idA"} {
		rt.containers[application.ContainerName("web", "img:"+tag, "")].ImageID = id
	}
	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
		attempts: map[string][]store.DeployAttempt{"web": {
			attemptAt("img:v3", now.Add(-5*time.Second)), attemptAt("img:v2", base.Add(3*time.Minute)), attemptAt("img:v1", base),
		}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34562" {
		t.Errorf("dial = %s, want the serving v2 release 127.0.0.1:34562", got)
	}
}

// A build that is still running must not pull traffic off the serving
// release just because its Ready condition predates the new attempt.
func TestDeploy_BuildInProgress_KeepsServingTarget(t *testing.T) {
	now := time.Now()
	base := now.Add(-time.Hour)
	desired := store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Domains: []string{"web.example.com"}}
	fake := newFakeRuntime()
	seedRelease(fake, "web", "img:v2", 34562, base.Add(3*time.Minute))
	seedRelease(fake, "web", "img:v1", 34561, base)
	rt := &startedRuntime{fakeRuntime: fake, started: map[string]time.Time{
		application.ContainerName("web", "img:v2", ""): base.Add(3 * time.Minute),
	}}
	st := &fakeStore{
		services: []store.DesiredService{desired},
		attempts: map[string][]store.DeployAttempt{"web": {
			{Image: "img:v3", Status: store.DeployAttemptStatusRunning, StartedAt: now.Add(-time.Second)},
			attemptAt("img:v2", base.Add(3*time.Minute)), attemptAt("img:v1", base),
		}},
		readyAt: map[string]time.Time{"web": now.Add(-time.Minute)},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34562" {
		t.Errorf("dial = %s, want the serving v2 target 127.0.0.1:34562", got)
	}
}
