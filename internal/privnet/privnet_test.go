package privnet

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEvaluate(t *testing.T) {
	apps := []App{{ID: "a", ProjectID: "p"}, {ID: "b", ProjectID: "p"}, {ID: "c", ProjectID: "p"}, {ID: "d", ProjectID: "q"}}
	nets := []Network{
		{ID: "n1", Name: "front", Members: []string{"a", "b"}},
		{ID: "n2", Name: "back", Members: []string{"c"}},
	}
	cases := []struct {
		name  string
		rules []Rule
		a, b  string
		want  bool
		via   string
	}{
		{"same network", nil, "a", "b", true, ViaNetwork},
		{"different networks blocked", nil, "a", "c", false, ViaNone},
		{"rule allows", []Rule{{ID: "r1", From: "c", To: "a"}}, "a", "c", true, ViaRule},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *Edge
			for _, e := range Evaluate(apps, nets, tc.rules) {
				if e.A == tc.a && e.B == tc.b {
					e := e
					got = &e
				}
			}
			if got == nil {
				t.Fatalf("no edge for %s-%s", tc.a, tc.b)
			}
			if got.Allowed != tc.want || got.Via != tc.via {
				t.Fatalf("edge = %+v, want allowed=%v via=%q", *got, tc.want, tc.via)
			}
		})
	}
}

func TestEvaluateDefaultDenyNeedsRule(t *testing.T) {
	apps := []App{{ID: "a"}, {ID: "b"}}
	nets := []Network{{ID: "n", Name: "vpc", DefaultDeny: true, Members: []string{"a", "b"}}}
	if e := Evaluate(apps, nets, nil); len(e) != 1 || e[0].Allowed {
		t.Fatalf("default-deny members must be blocked, got %+v", e)
	}
	if e := Evaluate(apps, nets, []Rule{{ID: "r", From: "a", To: "b"}}); len(e) != 1 || !e[0].Allowed {
		t.Fatalf("rule must allow, got %+v", e)
	}
}

func TestMeshHidden(t *testing.T) {
	nets := []Network{{ID: "n", DefaultDeny: true, Members: []string{"a", "b"}}}
	h := MeshHidden(nets, []Rule{{ID: "r", From: "a", To: "x"}})
	if h["a"] || !h["b"] {
		t.Fatalf("hidden = %v, want only b", h)
	}
}

type fakeSource struct {
	apps  []store.App
	nets  []store.PrivateNetwork
	rules []store.PrivateNetworkRule
}

func (f fakeSource) ListApps(context.Context) ([]store.App, error) { return f.apps, nil }
func (f fakeSource) ListPrivateNetworks(context.Context) ([]store.PrivateNetwork, error) {
	return f.nets, nil
}
func (f fakeSource) ListPrivateNetworkRules(context.Context) ([]store.PrivateNetworkRule, error) {
	return f.rules, nil
}

type fakeRuntime struct {
	containers []docker.ContainerState
	networks   map[string]bool
	removed    []string
}

func (f *fakeRuntime) EnsureNetwork(_ context.Context, name string) (string, error) {
	f.networks[name] = true
	return name, nil
}
func (f *fakeRuntime) RemoveNetwork(_ context.Context, name string) error {
	delete(f.networks, name)
	f.removed = append(f.removed, name)
	return nil
}
func (f *fakeRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	var out []docker.NetworkInfo
	for n := range f.networks {
		out = append(out, docker.NetworkInfo{Name: n})
	}
	return out, nil
}
func (f *fakeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return f.containers, nil
}
func (f *fakeRuntime) NetworkConnect(_ context.Context, network, id string) error {
	for i := range f.containers {
		if f.containers[i].ID == id {
			f.containers[i].Networks = append(f.containers[i].Networks, docker.ContainerNetwork{Name: network})
		}
	}
	return nil
}
func (f *fakeRuntime) NetworkDisconnect(_ context.Context, network, id string, _ bool) error {
	for i := range f.containers {
		if f.containers[i].ID != id {
			continue
		}
		kept := f.containers[i].Networks[:0]
		for _, n := range f.containers[i].Networks {
			if n.Name != network {
				kept = append(kept, n)
			}
		}
		f.containers[i].Networks = kept
	}
	return nil
}

func ctr(id, app string) docker.ContainerState {
	return docker.ContainerState{ID: id, Running: true, Networks: []docker.ContainerNetwork{{Name: "t-app-" + app}}}
}

func TestControllerIsolatesUntilRuleAllows(t *testing.T) {
	rt := &fakeRuntime{networks: map[string]bool{}, containers: []docker.ContainerState{ctr("c1", "a"), ctr("c2", "b")}}
	src := fakeSource{
		apps: []store.App{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}},
		nets: []store.PrivateNetwork{
			{ID: "n1", Name: "one", Members: []string{"a"}},
			{ID: "n2", Name: "two", Members: []string{"b"}},
		},
	}
	c := NewController(&src, rt, "t", "")
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	apps := src.apps
	if SharedNetworks("t", apps, rt.containers)[PairKey("a", "b")] {
		t.Fatal("apps in different networks must not share a Docker network")
	}

	src.rules = []store.PrivateNetworkRule{{ID: "r1", FromApp: "a", ToApp: "b"}}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !SharedNetworks("t", apps, rt.containers)[PairKey("a", "b")] {
		t.Fatal("rule must connect both apps")
	}

	src.rules = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if SharedNetworks("t", apps, rt.containers)[PairKey("a", "b")] {
		t.Fatal("removing the rule must disconnect the apps")
	}
	if len(rt.removed) != 1 || rt.removed[0] != "t-pl-r1" {
		t.Fatalf("removed = %v, want the rule network", rt.removed)
	}
}
