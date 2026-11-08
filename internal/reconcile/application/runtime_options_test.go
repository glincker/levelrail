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

func application0Name(svc *store.DesiredService) string {
	return ReplicaContainerName(svc.Name, NameImage(*svc), svc.RestartNonce, 0)
}
