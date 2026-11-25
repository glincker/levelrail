package application

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToContainerSpec_RuntimeOptions(t *testing.T) {
	on, to := true, 15
	d := &store.DesiredService{Name: "web", Image: "nginx:1", Runtime: &spec.RuntimeOptions{Init: &on, StopTimeout: &to, PidsLimit: 100,
		DNS: []string{"1.1.1.1"}, Ulimits: map[string]spec.Ulimit{"nofile": {Soft: 1, Hard: 2}}}}
	s, err := toContainerSpec("web-1", d)
	if err != nil || !s.Init || *s.StopTimeoutSeconds != 15 || s.PidsLimit != 100 || s.DNS[0] != "1.1.1.1" || s.Ulimits[0].Name != "nofile" {
		t.Fatalf("mapping wrong: %+v %v", s, err)
	}
	s, _ = toContainerSpec("web-1", &store.DesiredService{Name: "web", Image: "nginx:1"})
	if s.NeedsRuntimeCapability() || s.DNS != nil {
		t.Fatal("no runtime block must leave the spec untouched")
	}
}

func TestRuntimeOptions_ChangeRecreatesInPlaceAndUnchangedDoesNot(t *testing.T) {
	to := 10
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1, Runtime: &spec.RuntimeOptions{StopTimeout: &to}}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	name := rt.lastCreateSpec.Name
	creates := rt.createCalls
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates {
		t.Fatalf("unchanged options recreated the container (%d creates)", rt.createCalls-creates)
	}
	if got := application0Name(svc); got != name {
		t.Fatalf("name = %q, want %q", got, name)
	}

	to2 := 30
	svc.Runtime = &spec.RuntimeOptions{StopTimeout: &to2}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates+1 || rt.lastCreateSpec.Name != name {
		t.Fatalf("changed option: creates %d name %q, want one recreate under %q", rt.createCalls-creates, rt.lastCreateSpec.Name, name)
	}
	if !strings.HasPrefix(docker.RuntimeOptionsLabelKey, spec.ReservedLabelPrefix) {
		t.Fatal("label key must be reserved")
	}
}

func TestRuntimeOptions_IOCapChangeRecreates(t *testing.T) {
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	creates := rt.createCalls
	svc.Resources = &store.ServiceResources{IOReadBps: 50 << 20, IOWriteBps: 20 << 20}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates+1 {
		t.Fatalf("new io caps: %d creates, want 1", rt.createCalls-creates)
	}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates+1 {
		t.Fatal("unchanged io caps recreated again")
	}
}

func TestRuntimeOptions_LegacyLabelWithoutIOTermDoesNotRecreate(t *testing.T) {
	to := 30
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1,
		Runtime:   &spec.RuntimeOptions{StopTimeout: &to},
		Resources: &store.ServiceResources{IOReadBps: 50 << 20}}
	legacy := hashRuntimeOptionsKey(runtimeOptionsBaseKey(svc))
	st := &docker.ContainerState{Labels: map[string]string{docker.RuntimeOptionsLabelKey: legacy}}
	if runtimeOptionsDrifted(st, svc) {
		t.Fatal("old-formula label must count as converged")
	}
	st.Labels[docker.RuntimeOptionsLabelKey] = "deadbeef"
	if !runtimeOptionsDrifted(st, svc) {
		t.Fatal("unrelated label must drift")
	}
	delete(st.Labels, docker.RuntimeOptionsLabelKey)
	if !runtimeOptionsDrifted(st, svc) {
		t.Fatal("no label cannot be told from never capped, so a cap must apply")
	}
}

func TestRuntimeOptions_IOCapRemovalRecreatesOnce(t *testing.T) {
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1, Resources: &store.ServiceResources{IOReadBps: 1 << 20}}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	creates := rt.createCalls
	svc.Resources = nil
	for i := 0; i < 2; i++ {
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if rt.createCalls != creates+1 {
		t.Fatalf("cap removal: %d creates, want 1", rt.createCalls-creates)
	}
}

func TestRuntimeOptions_ZeroResourcesEqualsNil(t *testing.T) {
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	creates := rt.createCalls
	svc.Resources = &store.ServiceResources{}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates {
		t.Fatal("zero resources recreated against nil")
	}
}

func TestRuntimeOptions_RecreateStrategyPicksUpCapChangeGracefully(t *testing.T) {
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1, Strategy: "recreate"}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	creates, stops := rt.createCalls, rt.stopCalls
	svc.Resources = &store.ServiceResources{IOWriteBps: 1 << 20}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates+1 {
		t.Fatalf("recreate strategy: %d creates, want 1", rt.createCalls-creates)
	}
	if rt.stopCalls <= stops {
		t.Fatal("drift recreate must stop gracefully before removing")
	}
}

func application0Name(svc *store.DesiredService) string {
	return ReplicaContainerName(svc.Name, NameImage(*svc), svc.RestartNonce, 0)
}
