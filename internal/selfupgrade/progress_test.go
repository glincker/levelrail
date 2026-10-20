package selfupgrade

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type nopStore struct{}

func (nopStore) UpsertSelfUpgradeAttempt(context.Context, store.SelfUpgradeAttempt) error { return nil }

func TestApplyJournalsCurrentStepAndLog(t *testing.T) {
	h := newFakeHost(t)
	j := NewJournal(t.TempDir())
	d := h.deps()
	d.Journal = j
	got, err := Apply(context.Background(), d, req())
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "" || len(got.Log) < 2*len(got.Steps) {
		t.Fatalf("current=%q log=%d steps=%d", got.Current, len(got.Log), len(got.Steps))
	}
	list, _ := j.List()
	if len(list) != 1 || len(list[0].Log) == 0 {
		t.Fatalf("journal = %+v", list)
	}
}

func TestAddLogIsBounded(t *testing.T) {
	var a Attempt
	for i := 0; i < MaxLogLines+25; i++ {
		a.AddLog(time.Now(), StepGuard, fmt.Sprint(i))
	}
	if len(a.Log) != MaxLogLines || a.Log[0].Line != "25" {
		t.Fatalf("len=%d first=%q", len(a.Log), a.Log[0].Line)
	}
}

func TestImportJournalKeepsNewestFinished(t *testing.T) {
	j := NewJournal(t.TempDir())
	base := time.Now()
	for i := 0; i < KeepFinished+3; i++ {
		a := Attempt{ID: fmt.Sprintf("att-%d", i), Outcome: OutcomeSucceeded, StartedAt: base.Add(time.Duration(i) * time.Minute)}
		if err := j.Save(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Save(Attempt{ID: "att-run", Outcome: OutcomeRunning, StartedAt: base}); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportJournal(context.Background(), j, nopStore{}); err != nil {
		t.Fatal(err)
	}
	list, _ := j.List()
	if len(list) != KeepFinished+1 {
		t.Fatalf("kept %d files", len(list))
	}
}
