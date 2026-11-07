package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// agentNetRuntime mimics a remote agent: no NetworkAdmin, unlabeled networks,
// and a RemoveNetwork that refuses while an unmanaged container is attached.
type agentNetRuntime struct {
	*fakeRuntime
	nets    []docker.NetworkInfo
	inUse   map[string]bool
	removed []string
	calls   []string
}

func (a *agentNetRuntime) ListNetworksByPrefix(_ context.Context, prefix string) ([]docker.NetworkInfo, error) {
	var out []docker.NetworkInfo
	for _, n := range a.nets {
		if strings.HasPrefix(n.Name, prefix) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (a *agentNetRuntime) RemoveNetwork(_ context.Context, name string) error {
	a.calls = append(a.calls, name)
	if a.inUse[name] {
		return errors.New("rpc: docker: keep network: " + docker.ErrNetworkInUse.Error())
	}
	for i, n := range a.nets {
		if n.Name == name {
			a.nets = append(a.nets[:i], a.nets[i+1:]...)
			break
		}
	}
	a.removed = append(a.removed, name)
	return nil
}

type multiNodeStore struct {
	*store.DB
}

func (multiNodeStore) ListNodes(context.Context) ([]store.Node, error) {
	return []store.Node{{ID: "n1"}, {ID: "n2"}, {ID: "n3"}}, nil
}

func TestDeleteFinalizer_RemoveAppNetworkEveryNode(t *testing.T) {
	name := NetworkName("lr", "shop")
	foreign := map[string]string{appspec.InstanceLabelKey: "other"}
	nodes := map[string]*agentNetRuntime{
		"n0": {fakeRuntime: newFakeRuntime(0)},
		"n1": {fakeRuntime: newFakeRuntime(0), nets: []docker.NetworkInfo{{ID: "a", Name: name}, {ID: "b", Name: name + "-2"}}},
		"n2": {fakeRuntime: newFakeRuntime(0), nets: []docker.NetworkInfo{{ID: "c", Name: name}}, inUse: map[string]bool{name: true}},
		"n3": {fakeRuntime: newFakeRuntime(0), nets: []docker.NetworkInfo{{ID: "d", Name: name, Labels: foreign}}},
	}
	f := NewDeleteFinalizer(multiNodeStore{newFinalizerStore(t)}, func(id string) (docker.Runtime, error) { return nodes[id], nil }, nil,
		WithNetworkPrefix("lr"), WithInstanceID("inst-1"))
	for range 2 {
		if err := f.RemoveAppNetwork(context.Background(), "shop", "n0"); err != nil {
			t.Fatalf("RemoveAppNetwork() error = %v, want in-use and not-found tolerated", err)
		}
	}
	if got := nodes["n1"].removed; len(got) != 1 || got[0] != name {
		t.Errorf("n1 removed = %v, want only %s once across retries", got, name)
	}
	if len(nodes["n2"].removed) != 0 {
		t.Errorf("n2 removed %v, want a network with an unmanaged container kept", nodes["n2"].removed)
	}
	if len(nodes["n3"].calls) != 0 {
		t.Errorf("n3 RemoveNetwork calls = %v, want another instance's network untouched", nodes["n3"].calls)
	}
}

func TestNetworkCleanupAgentNodeGC(t *testing.T) {
	t.Setenv(EnvNetworkGCMaxRemovals, "2")
	rt := &agentNetRuntime{fakeRuntime: newFakeRuntime(0), inUse: map[string]bool{"p-app-busy": true}, nets: []docker.NetworkInfo{
		{Name: "p-app-busy"},
		{Name: "p-app-live"},
		{Name: "p-app-gone1"},
		{Name: "p-app-gone2"},
		{Name: "p-app-gone3"},
		{Name: "p-app-theirs", Labels: map[string]string{appspec.InstanceLabelKey: "them"}},
	}}
	c := NewNetworkCleanupController(&fakeAppLister{apps: []store.App{{ID: "live"}}}, rt, "p", "me").ForNode("n2")
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.removed) != 2 || rt.removed[0] != "p-app-gone1" || rt.removed[1] != "p-app-gone2" {
		t.Fatalf("removed %v, want the first two orphans (pass limit 2) with busy kept", rt.removed)
	}
	for _, n := range rt.calls {
		if n == "p-app-theirs" || n == "p-app-live" {
			t.Fatalf("RemoveNetwork(%q) called, want foreign and live networks untouched", n)
		}
	}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.removed) != 3 || rt.removed[2] != "p-app-gone3" {
		t.Fatalf("second pass removed %v, want p-app-gone3 next", rt.removed)
	}
	empty := &agentNetRuntime{fakeRuntime: newFakeRuntime(0), nets: []docker.NetworkInfo{{Name: "p-app-gone1"}}}
	if _, err := NewNetworkCleanupController(&fakeAppLister{}, empty, "p", "me").Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(empty.calls) != 0 {
		t.Fatalf("empty store removed unlabeled %v, want none", empty.calls)
	}
}
