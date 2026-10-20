package store

import (
	"context"
	"testing"
	"time"
)

func TestDeployTraceStepsUpsertKeepsStartAndCountsAttempts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Millisecond)

	run := DeployTraceStep{AttemptID: "dep_1", Key: "image_pull", Step: "image_pull", Status: TraceStatusRunning, Detail: "10%", StartedAt: start}
	if err := db.SaveDeployTraceStep(ctx, run); err != nil {
		t.Fatal(err)
	}
	done := run
	done.Status, done.Detail, done.EndedAt = TraceStatusDone, "100%", start.Add(1500*time.Millisecond)
	done.StartedAt = start.Add(time.Hour)
	if err := db.SaveDeployTraceStep(ctx, done); err != nil {
		t.Fatal(err)
	}
	other := DeployTraceStep{AttemptID: "dep_1", Step: "container_start", Status: TraceStatusDone, Reason: "x", StartedAt: start, EndedAt: start}
	if err := db.SaveDeployTraceStep(ctx, other); err != nil {
		t.Fatal(err)
	}

	got, err := db.ListDeployTraceSteps(ctx, "dep_1")
	if err != nil || len(got) != 2 {
		t.Fatalf("steps = %+v, err = %v, want 2", got, err)
	}
	pull := got[0]
	if pull.Status != TraceStatusDone || pull.Detail != "100%" || pull.Attempts != 2 {
		t.Errorf("pull = %+v, want done, 100%%, 2 attempts", pull)
	}
	if !pull.StartedAt.Equal(start) {
		t.Errorf("started_at = %v, want the first start %v kept", pull.StartedAt, start)
	}
	if pull.DurationMS < 1400 || pull.DurationMS > 1600 {
		t.Errorf("duration = %dms, want about 1500ms computed from the original start", pull.DurationMS)
	}
	if got[1].Key != "container_start" {
		t.Errorf("key = %q, want it to default to the step name", got[1].Key)
	}
	if other, _ := db.ListDeployTraceSteps(ctx, "dep_other"); len(other) != 0 {
		t.Errorf("other attempt steps = %v, want none", other)
	}
}

func TestTraceTargetForPicksNewestAttemptAndSeals(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	for i, id := range []string{"dep_old", "dep_new"} {
		if err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: id, ServiceName: "web", Image: "img:1", Status: DeployAttemptStatusRunning, StartedAt: now.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: "dep_failed", ServiceName: "web", Image: "img:1", Status: DeployAttemptStatusRunning, StartedAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_failed", DeployAttemptStatusFailed, now, "boom"); err != nil {
		t.Fatal(err)
	}

	id, sealed, err := db.TraceTargetFor(ctx, "web", "img:1")
	if err != nil || id != "dep_new" || sealed {
		t.Fatalf("target = %q sealed=%v err=%v, want dep_new unsealed (failed attempts are skipped)", id, sealed, err)
	}
	if err := db.SaveDeployTraceStep(ctx, DeployTraceStep{AttemptID: "dep_new", Step: TraceSummaryKey, Status: TraceStatusFailed, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, sealed, _ = db.TraceTargetFor(ctx, "web", "img:1"); sealed {
		t.Error("a failed summary sealed the trace, want only a done summary to seal it")
	}
	if err := db.SaveDeployTraceStep(ctx, DeployTraceStep{AttemptID: "dep_new", Step: TraceSummaryKey, Status: TraceStatusDone, StartedAt: now, EndedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, sealed, _ = db.TraceTargetFor(ctx, "web", "img:1"); !sealed {
		t.Error("a done summary did not seal the trace")
	}
	if id, _, err = db.TraceTargetFor(ctx, "web", "img:none"); err != nil || id != "" {
		t.Errorf("unknown image target = %q, err = %v, want empty", id, err)
	}
}
