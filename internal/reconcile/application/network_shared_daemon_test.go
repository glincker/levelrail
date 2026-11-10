package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// labelingRuntime is an agent-node runtime (no NetworkAdmin) that honors requested labels.
type labelingRuntime struct{ *fakeRuntime }

func (l *labelingRuntime) EnsureLabeledNetwork(ctx context.Context, name string, labels map[string]string) (string, error) {
	id, err := l.EnsureNetwork(ctx, name)
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, set := l.networkLabels[name]; !set {
		l.networkLabels[name] = labels
	}
	return id, err
}

func TestNetworkGCSharedDaemonTwoInstances(t *testing.T) {
	daemon := &labelingRuntime{newFakeRuntime(0)}
	// Instance B (older agent) owns an unlabeled network for its app minecraft.
	daemon.networks["lr-app-minecraft"] = "net-b"
	daemon.networks["lr-app-b-labeled"] = "net-b2"
	daemon.networkLabels["lr-app-b-labeled"] = map[string]string{appspec.InstanceLabelKey: "inst-b"}

	// Instance A deploys an app on the same daemon: its network must carry A's label.
	ctrlA := New("web", nil, daemon, WithNetworkPrefix("lr"), WithInstanceID("inst-a"))
	if err := ctrlA.ensureAppNetwork(context.Background(), &store.DesiredService{AppID: "shop"}); err != nil {
		t.Fatalf("ensureAppNetwork() error = %v", err)
	}
	if got := daemon.networkLabels["lr-app-shop"][appspec.InstanceLabelKey]; got != "inst-a" {
		t.Fatalf("A's network label = %q, want inst-a", got)
	}

	// A's app is deleted; A's store still has other apps (the old allowUnlabeled trigger).
	gcA := NewNetworkCleanupController(&fakeAppLister{apps: []store.App{{ID: "other"}}}, daemon, "lr", "inst-a").ForNode("shared")
	if _, err := gcA.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, ok := daemon.networks["lr-app-shop"]; ok {
		t.Error("A's own labeled orphan was kept, want it collected")
	}
	for _, n := range []string{"lr-app-minecraft", "lr-app-b-labeled"} {
		if _, ok := daemon.networks[n]; !ok {
			t.Errorf("%s was removed by instance A's GC, want it untouched", n)
		}
	}
}

func TestNetworkGCLocalNeverCollectsUnlabeled(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	views := ClassifyNetworks("lr", "inst-a", []docker.NetworkDetail{
		{Name: "lr-app-ghost", Created: old},
		{Name: "lr-app-theirs", Labels: map[string]string{appspec.InstanceLabelKey: "inst-b"}, Created: old},
		{Name: "lr-app-mine", Labels: map[string]string{appspec.InstanceLabelKey: "inst-a"}, Created: old},
	}, []store.App{{ID: "live"}}, time.Now(), time.Minute)
	want := map[string]string{"lr-app-ghost": NetworkKeepUnlabeled, "lr-app-theirs": NetworkKeepForeign, "lr-app-mine": NetworkRemovable}
	for _, v := range views {
		if v.Reason != want[v.Name] {
			t.Errorf("%s reason = %q, want %q", v.Name, v.Reason, want[v.Name])
		}
	}
}

func TestNetworkGCMaxRemovalsReadLive(t *testing.T) {
	rt := newFakeRuntime(0)
	for i := range 3 {
		n := fmt.Sprintf("lr-app-gone%d", i)
		rt.networks[n] = "id" + n
		rt.networkLabels[n] = map[string]string{appspec.InstanceLabelKey: "inst-a"}
	}
	c := NewNetworkCleanupController(&fakeAppLister{}, rt, "lr", "inst-a").WithMaxRemovals(func(context.Context) int { return 1 })
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rt.networks) != 2 {
		t.Fatalf("networks left = %d, want 2 (live cap of 1)", len(rt.networks))
	}
}

func TestDeleteAppNetworkAgentNodeExactNameRules(t *testing.T) {
	name := NetworkName("lr", "shop")
	db := newFinalizerStore(t)
	cases := []struct {
		name       string
		labels     map[string]string
		ranService bool
		inUse      bool
		removed    bool
	}{
		{"unlabeled on the service node", nil, true, false, true},
		{"unlabeled on another node", nil, false, false, false},
		{"own label on another node", map[string]string{appspec.InstanceLabelKey: "inst-a"}, false, false, true},
		{"other instance label", map[string]string{appspec.InstanceLabelKey: "inst-b"}, true, false, false},
		{"foreign container attached", nil, true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			rt.networks[name] = "net-1"
			rt.networks[name+"-2"] = "net-2"
			rt.networkLabels[name] = tc.labels
			if tc.inUse {
				rt.removeNetworkErr = docker.ErrNetworkInUse
			}
			f := NewDeleteFinalizer(db, func(string) (docker.Runtime, error) { return rt, nil }, nil, WithNetworkPrefix("lr"), WithInstanceID("inst-a"))
			if err := f.removeAppNetworkOn(context.Background(), "shop", "n1", tc.ranService); err != nil {
				t.Fatalf("removeAppNetworkOn() error = %v", err)
			}
			if _, kept := rt.networks[name]; kept == tc.removed {
				t.Errorf("network kept = %v, want removed = %v", kept, tc.removed)
			}
			if _, ok := rt.networks[name+"-2"]; !ok {
				t.Error("prefix sibling removed, want exact-name only")
			}
		})
	}
}
