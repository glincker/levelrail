package store

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

func TestChaosResultsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	older := chaos.Result{Scenario: chaos.KillContainer, Status: chaos.StatusPassed, Summary: "old", RanAt: time.Now().Add(-time.Hour)}
	newer := chaos.Result{
		Scenario: chaos.KillContainer, Status: chaos.StatusFailed, Summary: "new", RecoveryMS: 3525, Requests: 1200, RequestErrors: 141,
		Assertions: []chaos.Assertion{{Name: "a", Passed: false, Detail: "d"}}, RanAt: time.Now(),
	}
	for _, r := range []chaos.Result{older, newer} {
		if _, err := db.RecordChaosResult(ctx, r, "tester"); err != nil {
			t.Fatalf("RecordChaosResult() error = %v", err)
		}
	}
	got, err := db.ListChaosResults(ctx, 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("ListChaosResults() = %d rows, err %v", len(got), err)
	}
	if got[0].Summary != "new" || got[0].RecoveryMS != 3525 || len(got[0].Assertions) != 1 || got[0].Assertions[0].Detail != "d" {
		t.Fatalf("newest row = %+v", got[0])
	}
}
