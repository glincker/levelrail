package deploylog

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

func TestBuildingNotDoneBeforeCollect(t *testing.T) {
	r := NewRecorder(nil, nil)
	r.Start("a1")
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Build static site", Completed: true})
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Collect build output"})
	got, _ := r.StaticPhaseStates("a1")
	if got[PhaseBuilding] != "running" {
		t.Fatalf("building = %q, want running", got[PhaseBuilding])
	}
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Collect build output", Completed: true})
	got, _ = r.StaticPhaseStates("a1")
	if got[PhaseBuilding] != "done" {
		t.Fatalf("building = %q, want done", got[PhaseBuilding])
	}
}
