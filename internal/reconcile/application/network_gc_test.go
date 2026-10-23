package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestClassifyNetworks(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)
	own := map[string]string{appspec.InstanceLabelKey: "me"}
	other := map[string]string{appspec.InstanceLabelKey: "them"}
	apps := []store.App{{ID: "live"}}
	tests := []struct {
		name     string
		d        docker.NetworkDetail
		apps     []store.App
		want     string
		removed  bool
		orphaned bool
	}{
		{"owner exists", docker.NetworkDetail{Name: "p-app-live", Labels: own, Created: old}, apps, NetworkKeepOwned, false, false},
		{"orphan removable", docker.NetworkDetail{Name: "p-app-gone", Labels: own, Created: old}, apps, NetworkRemovable, true, true},
		{"orphan in use", docker.NetworkDetail{Name: "p-app-gone", Labels: own, Created: old, Attached: 1}, apps, NetworkKeepInUse, false, true},
		{"orphan within grace", docker.NetworkDetail{Name: "p-app-gone", Labels: own, Created: now.Add(-time.Minute)}, apps, NetworkKeepGrace, false, true},
		{"foreign instance", docker.NetworkDetail{Name: "p-app-gone", Labels: other, Created: old}, apps, NetworkKeepForeign, false, false},
		{"legacy unlabeled removable", docker.NetworkDetail{Name: "p-app-gone", Created: old}, apps, NetworkRemovable, true, true},
		{"legacy unlabeled, empty store", docker.NetworkDetail{Name: "p-app-gone", Created: old}, nil, NetworkKeepNoApps, false, true},
		{"not our pattern", docker.NetworkDetail{Name: "other-app-gone", Created: old}, apps, "", false, false},
	}
	for _, tc := range tests {
		got := ClassifyNetworks("p", "me", []docker.NetworkDetail{tc.d}, tc.apps, now, 10*time.Minute)
		if tc.want == "" {
			if len(got) != 0 {
				t.Errorf("%s: want skipped", tc.name)
			}
			continue
		}
		if len(got) != 1 || got[0].Reason != tc.want || got[0].Removable != tc.removed || got[0].Orphaned != tc.orphaned {
			t.Errorf("%s: got %+v", tc.name, got)
		}
	}
}

type adminRuntime struct {
	*fakeRuntime
	details []docker.NetworkDetail
	removed []string
	failOn  string
}

func (r *adminRuntime) DescribeNetworks(context.Context, string) ([]docker.NetworkDetail, error) {
	return r.details, nil
}

func (r *adminRuntime) RemoveNetworkIfEmpty(_ context.Context, name string) (bool, error) {
	if name == r.failOn {
		return false, errors.New("boom")
	}
	r.removed = append(r.removed, name)
	return true, nil
}

func (r *adminRuntime) NetworkPoolUsage(context.Context) (docker.PoolUsage, error) {
	return docker.PoolUsage{}, nil
}

func TestNetworkCleanupGCNeverTouchesForeignOrInUse(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	own := map[string]string{appspec.InstanceLabelKey: "me"}
	rt := &adminRuntime{fakeRuntime: newFakeRuntime(0), details: []docker.NetworkDetail{
		{Name: "p-app-live", Labels: own, Created: old},
		{Name: "p-app-gone", Labels: own, Created: old},
		{Name: "p-app-busy", Labels: own, Created: old, Attached: 2},
		{Name: "p-app-theirs", Labels: map[string]string{appspec.InstanceLabelKey: "them"}, Created: old},
		{Name: "p-app-fresh", Labels: own, Created: time.Now()},
	}}
	c := NewNetworkCleanupController(&fakeAppLister{apps: []store.App{{ID: "live"}}}, rt, "p", "me")
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.removed) != 1 || rt.removed[0] != "p-app-gone" {
		t.Fatalf("removed %v, want only p-app-gone", rt.removed)
	}
}

func TestCreateFailureReasonPoolExhausted(t *testing.T) {
	err := errors.Join(errors.New("ensure network"), &docker.PoolExhaustedError{Pool: "10.88.0.0/14", InUse: 1024, Capacity: 1024})
	if got := createFailureReason(err); got != "NetworkPoolExhausted" {
		t.Fatalf("reason %q", got)
	}
}

type selectiveAdminRuntime struct {
	*adminRuntime
	inspected []string
}

func (r *selectiveAdminRuntime) DescribeNetworksSelective(_ context.Context, _ string, needAttached func(string, map[string]string) bool) ([]docker.NetworkDetail, error) {
	for _, d := range r.details {
		if needAttached(d.Name, d.Labels) {
			r.inspected = append(r.inspected, d.Name)
		}
	}
	return r.details, nil
}

func TestNetworkCleanupInspectsOnlyOrphansOfThisInstance(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	own := map[string]string{appspec.InstanceLabelKey: "me"}
	rt := &selectiveAdminRuntime{adminRuntime: &adminRuntime{fakeRuntime: newFakeRuntime(0), details: []docker.NetworkDetail{
		{Name: "p-app-live", Labels: own, Created: old},
		{Name: "p-app-live2", Labels: own, Created: old},
		{Name: "p-app-gone", Labels: own, Created: old},
		{Name: "p-app-theirs", Labels: map[string]string{appspec.InstanceLabelKey: "them"}, Created: old},
	}}}
	c := NewNetworkCleanupController(&fakeAppLister{apps: []store.App{{ID: "live"}, {ID: "live2"}}}, rt, "p", "me")
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.inspected) != 1 || rt.inspected[0] != "p-app-gone" {
		t.Errorf("inspected %v, want only the orphan p-app-gone", rt.inspected)
	}
	if len(rt.removed) != 1 || rt.removed[0] != "p-app-gone" {
		t.Errorf("removed %v, want only p-app-gone", rt.removed)
	}
}
