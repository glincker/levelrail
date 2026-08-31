package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeScheduledTaskSource is an in-memory ScheduledTaskSource, the same
// hand-written-fake pattern every other package in this codebase uses
// instead of a mocking framework.
type fakeScheduledTaskSource struct {
	tasks map[string]store.ScheduledTask
}

func newFakeScheduledTaskSource(tasks ...store.ScheduledTask) *fakeScheduledTaskSource {
	m := make(map[string]store.ScheduledTask, len(tasks))
	for _, task := range tasks {
		m[task.ID] = task
	}
	return &fakeScheduledTaskSource{tasks: m}
}

func (f *fakeScheduledTaskSource) GetScheduledTask(_ context.Context, id string) (store.ScheduledTask, error) {
	task, ok := f.tasks[id]
	if !ok {
		return store.ScheduledTask{}, store.ErrScheduledTaskNotFound
	}
	return task, nil
}

func TestEvaluateScheduledTaskFailure_BelowThreshold_NotFiring(t *testing.T) {
	tasks := newFakeScheduledTaskSource(store.ScheduledTask{ID: "sct_1", ConsecutiveFailures: 2})
	r := Rule{ID: "r1", Kind: KindScheduledTaskFailure, ScheduledTaskID: "sct_1", RestartCountThreshold: 3, Enabled: true}

	got, notice, err := EvaluateScheduledTaskFailure(context.Background(), tasks, r, time.Now())
	if err != nil {
		t.Fatalf("EvaluateScheduledTaskFailure() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: 2 consecutive failures is under the threshold of 3")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty when not firing", notice)
	}
	if got.LastValue == nil || *got.LastValue != 2 {
		t.Errorf("LastValue = %v, want 2", got.LastValue)
	}
}

func TestEvaluateScheduledTaskFailure_MeetsThreshold_Fires(t *testing.T) {
	tasks := newFakeScheduledTaskSource(store.ScheduledTask{
		ID: "sct_1", Command: []string{"./cleanup.sh"}, ConsecutiveFailures: 3, LastRunStatus: store.ScheduledTaskStatusFailed,
	})
	r := Rule{ID: "r1", Kind: KindScheduledTaskFailure, ScheduledTaskID: "sct_1", RestartCountThreshold: 3, Enabled: true}

	got, notice, err := EvaluateScheduledTaskFailure(context.Background(), tasks, r, time.Now())
	if err != nil {
		t.Fatalf("EvaluateScheduledTaskFailure() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: 3 consecutive failures meets the threshold of 3")
	}
	if notice == "" {
		t.Error("notice is empty, want a non-empty failure summary when firing")
	}
}

func TestEvaluateScheduledTaskFailure_TaskDeleted_GoesQuietNoError(t *testing.T) {
	tasks := newFakeScheduledTaskSource() // empty: the watched task no longer exists
	firingSince := time.Now().Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindScheduledTaskFailure, ScheduledTaskID: "sct_gone", RestartCountThreshold: 3,
		Enabled: true, Firing: true, FiringSince: &firingSince}

	got, notice, err := EvaluateScheduledTaskFailure(context.Background(), tasks, r, time.Now())
	if err != nil {
		t.Fatalf("EvaluateScheduledTaskFailure() error = %v, want nil for a deleted task", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false once the watched task is gone")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
}

func TestEngine_Tick_ScheduledTaskFailureFires_NotifiesOnce(t *testing.T) {
	tasks := newFakeScheduledTaskSource(store.ScheduledTask{
		ID: "sct_1", Command: []string{"./backup.sh"}, ConsecutiveFailures: 3, LastRunStatus: store.ScheduledTaskStatusFailed,
	})
	r := Rule{ID: "r1", Kind: KindScheduledTaskFailure, ResourceID: "service:web", ScheduledTaskID: "sct_1",
		RestartCountThreshold: 3, Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngineWithTasks(rules, nil, nil, nil, tasks, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("Notify called %d times, want 1", len(calls))
	}
	if calls[0].TaskFailureNotice == "" {
		t.Error("TaskFailureNotice is empty on a firing event, want a non-empty summary")
	}
	if !rules.get("r1").Firing {
		t.Error("persisted state Firing = false, want true")
	}
}

func TestEngine_Tick_ScheduledTaskFailureResolved_NoNotice(t *testing.T) {
	tasks := newFakeScheduledTaskSource(store.ScheduledTask{ID: "sct_1", ConsecutiveFailures: 0, LastRunStatus: store.ScheduledTaskStatusSuccess})
	firingSince := time.Now().Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindScheduledTaskFailure, ResourceID: "service:web", ScheduledTaskID: "sct_1",
		RestartCountThreshold: 3, Enabled: true, Firing: true, FiringSince: &firingSince}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngineWithTasks(rules, nil, nil, nil, tasks, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 || !calls[0].Resolved {
		t.Fatalf("calls = %+v, want one Resolved=true event", calls)
	}
	if calls[0].TaskFailureNotice != "" {
		t.Errorf("TaskFailureNotice = %q, want empty on a resolved event", calls[0].TaskFailureNotice)
	}
}

func TestEngine_Tick_ScheduledTaskFailure_NoSourceConfigured_Skipped(t *testing.T) {
	r := Rule{ID: "r1", Kind: KindScheduledTaskFailure, ResourceID: "service:web", ScheduledTaskID: "sct_1",
		RestartCountThreshold: 3, Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngine(rules, nil, nil, nil, spy) // no scheduled task source wired

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 0 {
		t.Errorf("Notify called %d times with no scheduled task source configured, want 0", len(calls))
	}
}
