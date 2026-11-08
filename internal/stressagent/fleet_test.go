package stressagent

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestRuntimeLifecycle(t *testing.T) {
	ctx := context.Background()
	r := NewRuntime()
	id, err := r.Create(ctx, docker.ContainerSpec{Name: "app-1", Image: "x:1"})
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := r.InspectByName(ctx, "app-1"); st == nil || st.Running {
		t.Fatalf("created container must exist and be stopped: %+v", st)
	}
	if err := r.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.InspectByName(ctx, "app-1"); !st.Running {
		t.Fatal("started container must be running")
	}
	if got, _ := r.ListByPrefix(ctx, "app-"); len(got) != 1 {
		t.Fatalf("prefix list: %d", len(got))
	}
	if err := r.Remove(ctx, id, true); err != nil || r.Count() != 0 {
		t.Fatalf("remove: %v count %d", err, r.Count())
	}
	if st, err := r.InspectByName(ctx, "app-1"); st != nil || err != nil {
		t.Fatal("absent container must be (nil, nil)")
	}
}
