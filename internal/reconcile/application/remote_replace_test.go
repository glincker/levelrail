package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// agentShapedRuntime reports containers the way the agent transport does:
// no network attachments, labels only when the daemon returned some.
type agentShapedRuntime struct{ *fakeRuntime }

func agentShaped(cs docker.ContainerState) docker.ContainerState {
	cs.Networks = nil
	cs.LabelsUnknown = len(cs.Labels) == 0
	return cs
}

func (r agentShapedRuntime) ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	all, err := r.fakeRuntime.ListByPrefix(ctx, prefix)
	for i := range all {
		all[i] = agentShaped(all[i])
	}
	return all, err
}

func (r agentShapedRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	cs, err := r.fakeRuntime.InspectByName(ctx, name)
	if cs == nil || err != nil {
		return cs, err
	}
	out := agentShaped(*cs)
	return &out, nil
}

// remotePass mirrors production: a fresh controller and pass snapshot per
// pass, keyed by the remote node, around one shared drain tracker.
func remotePass(t *testing.T, rt *fakeRuntime, tracker *DrainTracker, hold time.Duration, at time.Time, desired store.DesiredService) {
	t.Helper()
	snap := NewPassSnapshot(time.Minute)
	opts := []Option{
		WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1}, nil), WithGateRecorder(&fakeGateRecorder{}),
		WithGateTick(10 * time.Millisecond), WithReadyBudget(2 * time.Second), WithDrainTracker(tracker),
		WithPreviousReleaseHold(hold), WithInstanceID("cp-a"), WithPassSnapshot(snap, "oci1"),
	}
	c := New("web", &fakeStore{svc: &desired}, agentShapedRuntime{rt}, opts...)
	c.now = func() time.Time { return at }
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() at %s error = %v", at.Format(time.TimeOnly), err)
	}
	rt.mu.Lock()
	for _, cs := range rt.containers {
		if cs.Created.IsZero() {
			cs.Created = at
		}
	}
	rt.mu.Unlock()
}

func runningOwned(rt *fakeRuntime) []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var out []string
	for name, cs := range rt.containers {
		if cs.Running && ownsContainer("web", name) && cs.Labels[appspec.InstanceLabelKey] != "cp-b" {
			out = append(out, name)
		}
	}
	return out
}

// A config change (a volume attached bumps the restart nonce) on a remote
// node retires the previous replica once the hold ends, and never touches a
// same-named container another control plane owns on that shared daemon.
func TestRemoteReplace_PreviousReplicaRetiredAfterHold(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	for _, hold := range []time.Duration{0, 3 * time.Minute} {
		t.Run(hold.String(), func(t *testing.T) {
			rt := newFakeRuntime(serverPort(t, srv))
			tracker := NewDrainTracker()
			desired := *gateDesired()
			desired.Image = "img:v1"
			a := ContainerName("web", NameImage(desired), "")
			b := ContainerName("web", NameImage(desired), "vol-1")
			foreign := ContainerName("web", "img:v0", "")
			rt.containers[foreign] = &docker.ContainerState{ID: "foreign", Name: foreign, Running: true,
				Labels: map[string]string{appspec.InstanceLabelKey: "cp-b"}}
			for _, name := range []string{a, b} {
				rt.setExitState(name, &docker.ExitState{Running: true})
			}
			t0 := time.Now().Truncate(time.Second)

			remotePass(t, rt, tracker, hold, t0.Add(-time.Hour), desired)
			desired.RestartNonce = "vol-1"
			remotePass(t, rt, tracker, hold, t0, desired)
			if cs, _ := rt.InspectByName(context.Background(), b); cs == nil || !cs.Running {
				t.Fatalf("replacement %s = %+v, want running", b, cs)
			}
			remotePass(t, rt, tracker, hold, t0.Add(hold+time.Second), desired)
			remotePass(t, rt, tracker, hold, t0.Add(hold+2*time.Second), desired)
			if got := runningOwned(rt); len(got) != 1 || got[0] != b {
				t.Fatalf("running replicas at steady state = %v, want only %s", got, b)
			}
			if cs, _ := rt.InspectByName(context.Background(), foreign); cs == nil || !cs.Running {
				t.Fatalf("foreign instance container %s = %+v, want untouched", foreign, cs)
			}

			desired.Suspended = true
			remotePass(t, rt, tracker, hold, t0.Add(hold+3*time.Second), desired)
			if got := runningOwned(rt); len(got) != 0 {
				t.Fatalf("running after stop = %v, want none", got)
			}
			if cs, _ := rt.InspectByName(context.Background(), foreign); cs == nil {
				t.Fatalf("stop removed the foreign instance container %s", foreign)
			}
		})
	}
}
