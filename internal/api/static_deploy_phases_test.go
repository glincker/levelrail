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
