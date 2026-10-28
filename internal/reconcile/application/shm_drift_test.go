package application

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestShmSizeLabelKey_IsReserved(t *testing.T) {
	if !strings.HasPrefix(docker.ShmSizeLabelKey, appspec.ReservedLabelPrefix) {
		t.Fatalf("%q must sit under %q so operator labels cannot spoof it", docker.ShmSizeLabelKey, appspec.ReservedLabelPrefix)
	}
}

func TestShmSize_CreateCarriesSizeAndRecreatesOnChange(t *testing.T) {
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1,
		Labels:    map[string]string{docker.ShmSizeLabelKey: "1", "keep": "me"},
		Resources: &store.ServiceResources{ShmSizeBytes: 1 << 30}}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: svc}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	got := rt.lastCreateSpec
	if got.ShmSizeBytes != 1<<30 || got.Labels[docker.ShmSizeLabelKey] != "1073741824" || got.Labels["keep"] != "me" {
		t.Fatalf("create spec shm = %d labels = %v, want 1 GiB stamped over the user copy", got.ShmSizeBytes, got.Labels)
	}

	creates := rt.createCalls
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates {
		t.Fatalf("unchanged shm_size recreated the container (%d creates)", rt.createCalls)
	}

	svc.Resources = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != creates+1 || rt.lastCreateSpec.ShmSizeBytes != 0 {
		t.Fatalf("dropping shm_size: creates %d spec shm %d, want one recreate at the default", rt.createCalls-creates, rt.lastCreateSpec.ShmSizeBytes)
	}
	if _, ok := rt.lastCreateSpec.Labels[docker.ShmSizeLabelKey]; ok {
		t.Fatalf("label must be cleared when shm_size is unset: %v", rt.lastCreateSpec.Labels)
	}
}

func TestShmDrifted(t *testing.T) {
	want := &store.DesiredService{Resources: &store.ServiceResources{ShmSizeBytes: 512}}
	tests := []struct {
		name    string
		state   *docker.ContainerState
		desired *store.DesiredService
		drift   bool
	}{
		{"no container", nil, want, false},
		{"labels unknown on an old agent", &docker.ContainerState{LabelsUnknown: true}, want, false},
		{"legacy container without the label", &docker.ContainerState{Labels: map[string]string{}}, want, true},
		{"same size", &docker.ContainerState{Labels: map[string]string{docker.ShmSizeLabelKey: "512"}}, want, false},
		{"default on both sides", &docker.ContainerState{Labels: map[string]string{}}, &store.DesiredService{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shmDrifted(tt.state, tt.desired); got != tt.drift {
				t.Fatalf("shmDrifted() = %v, want %v", got, tt.drift)
			}
		})
	}
}
