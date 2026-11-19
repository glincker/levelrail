package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStaticDeployPhases(t *testing.T) {
	rt, _ := newTestRouter(t)
	now := time.Now()
	ok := store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &now}
	got := rt.staticDeployPhases(ok, true)
	if len(got) != 4 || got[0].Name != "cloning" || got[3].Name != "live" || got[3].State != "done" {
		t.Fatalf("succeeded phases = %+v", got)
	}
	if rt.staticDeployPhases(ok, false) != nil {
		t.Fatal("a non-static attempt must have no phases")
	}
	run := store.DeployAttempt{Status: store.DeployAttemptStatusRunning}
	if got := rt.staticDeployPhases(run, true); got[0].State != "pending" || got[3].State != "pending" {
		t.Fatalf("running phases = %+v", got)
	}
}

func TestStaticDeployPhasesFinished(t *testing.T) {
	rt, db := newTestRouter(t)
	now := time.Now()
	save := func(id, phase, status string) {
		t.Helper()
		row := store.DeployTraceStep{AttemptID: id, Key: "static_phase." + phase, Step: "static_phase", Status: status, Detail: phase, StartedAt: now}
		if err := db.SaveDeployTraceStep(t.Context(), row); err != nil {
			t.Fatal(err)
		}
	}
	states := func(a store.DeployAttempt) string {
		var s string
		for _, p := range rt.staticDeployPhases(a, true) {
			s += p.State + ","
		}
		return s
	}
	save("dep_f", "cloning", "done")
	save("dep_f", "building", "failed")
	failed := store.DeployAttempt{ID: "dep_f", Status: store.DeployAttemptStatusFailed, FinishedAt: &now}
	if got := states(failed); got != "done,failed,pending,pending," {
		t.Fatalf("failed states = %s", got)
	}
	// A failure outside a progress event leaves no failed row: the first phase that is not done fails.
	save("dep_g", "cloning", "done")
	save("dep_g", "building", "done")
	gap := store.DeployAttempt{ID: "dep_g", Status: store.DeployAttemptStatusFailed, FinishedAt: &now}
	if got := states(gap); got != "done,done,failed,pending," {
		t.Fatalf("gap states = %s", got)
	}
	ok := store.DeployAttempt{ID: "dep_f", Status: store.DeployAttemptStatusSucceeded, FinishedAt: &now}
	if got := states(ok); got != "done,done,done,done," {
		t.Fatalf("succeeded states = %s", got)
	}
	none := store.DeployAttempt{ID: "dep_none", Status: store.DeployAttemptStatusFailed, FinishedAt: &now}
	if got := states(none); got != "unknown,unknown,unknown,pending," {
		t.Fatalf("no rows states = %s", got)
	}
}
