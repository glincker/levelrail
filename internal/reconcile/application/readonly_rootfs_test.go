package application

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func boolPtr(b bool) *bool { return &b }

func TestReadOnlyRootfsLabelKey_IsReserved(t *testing.T) {
	if !strings.HasPrefix(docker.ReadOnlyRootfsLabelKey, appspec.ReservedLabelPrefix) {
		t.Fatalf("%q must sit under %q", docker.ReadOnlyRootfsLabelKey, appspec.ReservedLabelPrefix)
	}
}

func TestReadOnlyRootfs_DefaultStampsNothing(t *testing.T) {
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.lastCreateSpec.Labels[docker.ReadOnlyRootfsLabelKey]; ok {
		t.Fatalf("unset app carries the label: %v", rt.lastCreateSpec.Labels)
	}
}

func TestReadOnlyRootfs_RecreatesOnceAndSettles(t *testing.T) {
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	reconcile := func() {
		t.Helper()
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	reconcile()
	creates := rt.createCalls

	svc.Resources = &store.ServiceResources{ReadOnlyRootfs: boolPtr(true)}
	reconcile()
	if rt.createCalls != creates+1 || rt.lastCreateSpec.Labels[docker.ReadOnlyRootfsLabelKey] != "true" {
		t.Fatalf("enabling: creates %d labels %v, want one recreate stamped true", rt.createCalls-creates, rt.lastCreateSpec.Labels)
	}
	creates = rt.createCalls
	reconcile()
	reconcile()
	if rt.createCalls != creates {
		t.Fatalf("settled passes created %d containers, want 0", rt.createCalls-creates)
	}

	svc.Resources.ReadOnlyRootfs = boolPtr(false)
	reconcile()
	if rt.createCalls != creates+1 || rt.lastCreateSpec.Labels[docker.ReadOnlyRootfsLabelKey] != "false" {
		t.Fatalf("explicit false: creates %d labels %v", rt.createCalls-creates, rt.lastCreateSpec.Labels)
	}
	creates = rt.createCalls
	reconcile()
	if rt.createCalls != creates {
		t.Fatal("settled false recreated the container")
	}

	svc.Resources = nil
	reconcile()
	if rt.createCalls != creates+1 {
		t.Fatalf("back to inherit: creates %d, want one recreate", rt.createCalls-creates)
	}
	if _, ok := rt.lastCreateSpec.Labels[docker.ReadOnlyRootfsLabelKey]; ok {
		t.Fatal("label must be cleared on inherit")
	}
}

func TestReadOnlyRootfsDrifted(t *testing.T) {
	on := &store.DesiredService{Resources: &store.ServiceResources{ReadOnlyRootfs: boolPtr(true)}}
	tests := []struct {
		name    string
		state   *docker.ContainerState
		desired *store.DesiredService
		drift   bool
	}{
		{"no container", nil, on, false},
		{"labels unknown", &docker.ContainerState{LabelsUnknown: true}, on, false},
		{"legacy container", &docker.ContainerState{Labels: map[string]string{}}, on, true},
		{"same", &docker.ContainerState{Labels: map[string]string{docker.ReadOnlyRootfsLabelKey: "true"}}, on, false},
		{"flipped", &docker.ContainerState{Labels: map[string]string{docker.ReadOnlyRootfsLabelKey: "false"}}, on, true},
		{"inherit both sides", &docker.ContainerState{Labels: map[string]string{}}, &store.DesiredService{}, false},
		{"inherit but labelled", &docker.ContainerState{Labels: map[string]string{docker.ReadOnlyRootfsLabelKey: "true"}}, &store.DesiredService{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readOnlyRootfsDrifted(tt.state, tt.desired); got != tt.drift {
				t.Fatalf("drifted = %v, want %v", got, tt.drift)
			}
		})
	}
}
