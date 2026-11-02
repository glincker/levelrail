package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func runningCount(rt *fakeRuntime) int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	n := 0
	for _, cs := range rt.containers {
		if cs.Running {
			n++
		}
	}
	return n
}

// A stopped app with a stream port, a named volume and a past succeeded
// attempt must stay at zero containers on every pass: no revive path may bring it back.
func TestSuspended_StaysStoppedAcrossPasses(t *testing.T) {
	g := newGateServer(t, true)
	desired := takeoverDesired("img:v2", time.Second)
	desired.Suspended = true
	rig := newTakeoverRig(t, g, desired, true)
	rig.st.attempts = append(rig.st.attempts, store.DeployAttempt{ID: "d2", Image: "img:v2", Status: store.DeployAttemptStatusSucceeded})
	rig.rt.seed(rig.v2, true)
	rig.rt.containers[rig.v2].Image = "img:v2"
	c := rig.controller()
	for i := 0; i < 4; i++ {
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		if got := runningCount(rig.rt); got != 0 {
			t.Fatalf("pass %d: %d containers still running for a suspended app", i, got)
		}
		if res.Conditions[0].Reason != "Suspended" {
			t.Fatalf("pass %d: reason %q", i, res.Conditions[0].Reason)
		}
	}
	c.reviveLastReady(context.Background(), []string{rig.v2}, desired)
	if got := runningCount(rig.rt); got != 0 {
		t.Fatalf("revive started %d containers of a suspended app", got)
	}
}

// Three quick deploys inside the hold window leave the live release plus at
// most one held previous release, never an older one.
func TestRetire_AtMostOneHeldPreviousRelease(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	cfg := deploysafety.Effective{SuccessThreshold: 1, DrainTimeout: 10 * time.Second}
	p := clockedPass{t: t, rt: rt, tracker: NewDrainTracker(), cfg: cfg, hold: 5 * time.Minute}
	desired := *gateDesired()
	names := map[string]string{}
	for _, img := range []string{"img:v1", "img:v2", "img:v3"} {
		names[img] = ContainerName("web", img, "")
		rt.setExitState(names[img], &docker.ExitState{Running: true})
	}
	t0 := time.Now().Truncate(time.Second)
	for i, img := range []string{"img:v1", "img:v2", "img:v3"} {
		desired.Image = img
		p.run(t0.Add(time.Duration(i)*time.Minute), desired)
	}
	for _, at := range []time.Duration{3 * time.Minute, 4 * time.Minute} {
		p.run(t0.Add(at), desired)
		held := 0
		for _, img := range []string{"img:v1", "img:v2"} {
			if p.running(names[img]) {
				held++
			}
		}
		if held > 1 || p.running(names["img:v1"]) || !p.running(names["img:v3"]) {
			t.Fatalf("at %s: %d held previous releases (v1 running=%v), want at most v2 beside v3", at, held, p.running(names["img:v1"]))
		}
	}
}

// The revive paths refuse a suspended app even when a stopped older release is on disk.
func TestSuspended_ReviveRefusesStoppedOlderRelease(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "app:v3", Port: 80, Suspended: true}
	oldName := ContainerName("web", NameImage(store.DesiredService{Image: "app:v2"}), "")
	rt := newFakeRuntime(8080)
	rt.seed(oldName, false)
	rt.containers[oldName].Image = "app:v2"
	st := &historyStore{
		fakeStore: fakeStore{svc: desired},
		attempts:  []store.DeployAttempt{{ID: "d2", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded}},
	}
	c := New("web", st, rt, WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
	c.reviveIfDead(context.Background(), []string{"web-x"}, desired, false)
	if rt.containers[oldName].Running {
		t.Fatal("a suspended app's older release was revived")
	}
}

type retainedYes struct{}

func (retainedYes) NoteUnready(context.Context, string) (bool, bool, error) { return true, false, nil }
func (retainedYes) NoteReady(string)                                        {}
func (retainedYes) IsRetained(context.Context, string) bool                 { return true }

// Retained copies carry the reserved label so a sweep can find them without a database row.
func TestRetainedCopyContainersAreLabeled(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{Name: "web-aaaaaaa", Image: "img:v1", Port: 80, Replicas: 1}
	c := New("web-aaaaaaa", &fakeStore{svc: desired}, rt, WithRetainedDeploys(retainedYes{}), WithReadyBudget(time.Second))
	_, _ = c.Reconcile(context.Background())
	if rt.lastCreateSpec.Labels[appspec.DeployURLLabelKey] != "1" {
		t.Fatalf("labels = %v, want the deploy-url label", rt.lastCreateSpec.Labels)
	}
}
