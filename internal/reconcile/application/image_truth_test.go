package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// digestRuntime is fakeRuntime plus image IDs: ids maps an image reference
// to the local ID the node resolves it to, and a created container runs
// whatever its reference resolved to at create time.
type digestRuntime struct {
	*fakeRuntime
	ids        map[string]string
	inspectErr error
	now        time.Time
}

func newDigestRuntime(ids map[string]string) *digestRuntime {
	return &digestRuntime{fakeRuntime: newFakeRuntime(0), ids: ids, now: time.Now()}
}

func (d *digestRuntime) InspectImageID(_ context.Context, ref string) (string, error) {
	if d.inspectErr != nil {
		return "", d.inspectErr
	}
	return d.ids[ref], nil
}

func (d *digestRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	id, err := d.fakeRuntime.Create(ctx, spec)
	if err != nil {
		return id, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	cs := d.containers[spec.Name]
	cs.ImageID = d.ids[spec.Image]
	cs.Created = d.now
	return id, nil
}

// seedRunning adds a running container with explicit image identity.
func (d *digestRuntime) seedRunning(name, imageID string, created time.Time) {
	d.seed(name, true)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.containers[name].ImageID = imageID
	d.containers[name].Created = created
}

type fakeRollouts struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeRollouts) RecordRollout(_ context.Context, _, image, state, running string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, image+"|"+state+"|"+running)
	return nil
}

func (f *fakeRollouts) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

func TestNameImageFoldsLocalBuildID(t *testing.T) {
	a := store.DesiredService{Name: "web", Image: "web:abc", ImageID: "sha256:1", ImageIDRef: "web:abc"}
	b := a
	b.ImageID = "sha256:2"
	if ContainerName("web", NameImage(a), "") == ContainerName("web", NameImage(b), "") {
		t.Fatal("a rebuild of the same tag with new content must get a new container name")
	}
	legacy := store.DesiredService{Name: "web", Image: "web:abc"}
	if ContainerName("web", NameImage(legacy), "") != ContainerName("web", "web:abc", "") {
		t.Fatal("services without an image id must keep their existing container name")
	}
}

func TestReconcile_PinnedDigestServingIsReady(t *testing.T) {
	ref := "nginx:latest@sha256:new"
	rt := newDigestRuntime(map[string]string{ref: "sha256:cfgnew"})
	rec := &fakeRollouts{}
	desired := &store.DesiredService{Name: "web", Image: ref, Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt, WithRolloutRecorder(rec))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Fatalf("condition = %+v", cond)
	}
	if got := rec.last(); got != ref+"|serving|sha256:cfgnew" {
		t.Fatalf("rollout = %q", got)
	}
}

// The Dokploy 5496 class: the container runs old content while the deploy
// says it is done. Here the node's image for the pinned reference is not
// what the running container holds (a tampered or re-tagged image), and
// the controller must refuse to call it Ready.
func TestReconcile_RunningImageMismatchIsNeverReady(t *testing.T) {
	ref := "nginx:latest@sha256:new"
	rt := newDigestRuntime(map[string]string{ref: "sha256:cfgnew"})
	rec := &fakeRollouts{}
	desired := &store.DesiredService{Name: "web", Image: ref, Port: 80}
	target := ContainerName("web", NameImage(*desired), "")
	rt.seedRunning(target, "sha256:cfgold", time.Now())
	c := New("web", &fakeStore{svc: desired}, rt, WithRolloutRecorder(rec))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("a mismatched running image must surface an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "ImageDigestMismatch" {
		t.Fatalf("condition = %+v, want False/ImageDigestMismatch", cond)
	}
	if got := rec.last(); got != ref+"|mismatch|sha256:cfgold" {
		t.Fatalf("rollout = %q", got)
	}
}

func TestReconcile_LocalBuildRebuildReplacesContainer(t *testing.T) {
	rt := newDigestRuntime(map[string]string{"web:abc": "sha256:second"})
	old := store.DesiredService{Name: "web", Image: "web:abc", ImageID: "sha256:first", ImageIDRef: "web:abc", Port: 80}
	oldName := ContainerName("web", NameImage(old), "")
	rt.seedRunning(oldName, "sha256:first", time.Now().Add(-time.Hour))

	desired := old
	desired.ImageID = "sha256:second"
	c := New("web", &fakeStore{svc: &desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Deployed" {
		t.Fatalf("condition = %+v", cond)
	}
	names := rt.names()
	if len(names) != 1 || names[0] == oldName {
		t.Fatalf("containers = %v, want only the rebuilt one", names)
	}
}

// Half-succeeded: the new container was created and is ready, but the
// image inspect used to verify it fails. The deploy must not be reported
// Ready, and the old release must stay serving.
func TestReconcile_ImageInspectFailsKeepsOldRelease(t *testing.T) {
	rt := newDigestRuntime(map[string]string{})
	rt.inspectErr = errors.New("docker daemon unavailable")
	oldName := ContainerName("web", "nginx:latest@sha256:old", "")
	rt.seedRunning(oldName, "sha256:cfgold", time.Now().Add(-time.Hour))

	desired := &store.DesiredService{Name: "web", Image: "nginx:latest@sha256:new", Port: 80}
	rt.ids[desired.Image] = "sha256:cfgnew"
	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionFalse || cond.Reason != "ImageInspectFailed" {
		t.Fatalf("condition = %+v", cond)
	}
	found := false
	for _, n := range rt.names() {
		if n == oldName {
			found = true
		}
	}
	if !found {
		t.Fatal("old release was removed although the new one was never verified")
	}
}

func TestReconcile_UnpinnedLegacyImageIsNotVerified(t *testing.T) {
	rt := newDigestRuntime(map[string]string{"nginx:latest": "sha256:moved"})
	desired := &store.DesiredService{Name: "web", Image: "nginx:latest", Port: 80}
	rt.seedRunning(ContainerName("web", "nginx:latest", ""), "sha256:original", time.Now())
	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("legacy unpinned deploys keep today's behavior, got %+v", cond)
	}
}

func TestReconcile_PreviousReleaseHeldThenRemoved(t *testing.T) {
	rt := newDigestRuntime(map[string]string{})
	oldName := ContainerName("web", "img:v1", "")
	rt.seedRunning(oldName, "", time.Now().Add(-time.Hour))
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt, WithPreviousReleaseHold(5*time.Minute))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.count() != 2 {
		t.Fatalf("containers = %v, want new plus held previous release", rt.names())
	}
	// Instant rollback: pointing back at v1 reuses the held container.
	creates := rt.createCalls
	desired.Image = "img:v1"
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates {
		t.Fatal("rollback within the hold created a container instead of reusing the held one")
	}
	if rt.count() != 2 {
		t.Fatalf("containers after rollback = %v, want the rolled-back-from release held too", rt.names())
	}
	// Roll forward to the still-held v2, then age everything past the
	// hold: exactly one container remains.
	desired.Image = "img:v2"
	rt.mu.Lock()
	for _, cs := range rt.containers {
		cs.Created = cs.Created.Add(-time.Hour)
	}
	rt.mu.Unlock()
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.count() != 1 {
		t.Fatalf("containers after hold expiry = %v", rt.names())
	}
}

func TestHeldFilter(t *testing.T) {
	now := time.Now()
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithPreviousReleaseHold(time.Minute))
	target := ContainerName("web", "img:v2", "")
	prev := ContainerName("web", "img:v1", "")
	all := []docker.ContainerState{
		{ID: "1", Name: target, Running: true, Created: now.Add(-10 * time.Second)},
		{ID: "2", Name: prev, Running: true, Created: now.Add(-time.Hour)},
		{ID: "3", Name: target + "-r2", Running: true, Created: now.Add(-time.Hour)},
		{ID: "4", Name: ContainerName("web", "img:v0", ""), Running: false, Created: now.Add(-2 * time.Hour)},
	}
	due := c.heldFilter(all, []string{target}, now)
	ids := map[string]bool{}
	for _, cs := range due {
		ids[cs.ID] = true
	}
	if ids["2"] || !ids["3"] || !ids["4"] || ids["1"] {
		t.Fatalf("due = %v, want excess replica and stopped container only", ids)
	}
	if due := c.heldFilter(all, []string{target}, now.Add(2*time.Minute)); len(due) != 3 {
		t.Fatalf("after the hold, due = %d, want 3", len(due))
	}
}
