package backup

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type sweepRuntime struct {
	*fakeExecRuntime
	list    []docker.ContainerState
	removed []string
}

func (s *sweepRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return s.list, nil
}

func (s *sweepRuntime) Remove(_ context.Context, id string, _ bool) error {
	s.removed = append(s.removed, id)
	return nil
}

func TestHardenScratchAndSweepOrphans(t *testing.T) {
	r := &DrillRunner{}
	spec := docker.ContainerSpec{Name: "drill-x"}
	r.hardenScratch(&spec)
	if spec.NetworkMode != "none" || spec.Resources == nil || spec.Resources.MemoryBytes <= 0 || spec.Labels[drillScratchLabel] != "1" {
		t.Fatalf("scratch spec not hardened: %+v", spec)
	}

	rt := &sweepRuntime{fakeExecRuntime: &fakeExecRuntime{}, list: []docker.ContainerState{
		{ID: "a", Name: "drill-a", Labels: map[string]string{drillScratchLabel: "1"}},
		{ID: "b", Name: "drill-user-app"},
	}}
	r.Runtime = rt
	n, err := r.SweepOrphans(context.Background())
	if err != nil || n != 1 || len(rt.removed) != 1 || rt.removed[0] != "a" {
		t.Fatalf("sweep removed %d %v err %v, want only labelled orphan", n, rt.removed, err)
	}
}
