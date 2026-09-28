package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPipelineRunLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	p, err := db.SavePipeline(ctx, Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: "a", Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("save pipeline: %v", err)
	}
	p2, err := db.SavePipeline(ctx, Pipeline{ID: "ignored", AppName: "web", Name: "ci", YAML: "b", Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil || p2.ID != p.ID || p2.YAML != "b" {
		t.Fatalf("upsert kept id %q yaml %q err %v", p2.ID, p2.YAML, err)
	}

	r1, err := db.CreatePipelineRun(ctx, PipelineRun{ID: "r1", PipelineID: p.ID, AppName: "web", TriggerKind: "manual", Definition: "b", CreatedAt: now})
	if err != nil || r1.Number != 1 {
		t.Fatalf("run 1: %+v %v", r1, err)
	}
	r2, _ := db.CreatePipelineRun(ctx, PipelineRun{ID: "r2", PipelineID: p.ID, AppName: "web", TriggerKind: "push", Definition: "b", CreatedAt: now.Add(time.Second)})
	if r2.Number != 2 {
		t.Fatalf("run 2 number = %d", r2.Number)
	}

	jobs := []PipelineJob{{ID: "j1", RunID: "r1", Key: "test", DisplayName: "test", NeedsJSON: "[]", MatrixJSON: "{}", Status: PipelineStatusPending,
		Steps: []PipelineStep{{Index: 0, Name: "a", Kind: "run"}, {Index: 1, Name: "b", Kind: "run"}}}}
	if err := db.CreatePipelineJobs(ctx, jobs); err != nil {
		t.Fatalf("create jobs: %v", err)
	}
	code := 3
	if err := db.SetPipelineStepStatus(ctx, "j1", 1, PipelineStatusFailed, "exit 3", &code, 1, &now, &now); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListPipelineJobs(ctx, "r1")
	if err != nil || len(got) != 1 || len(got[0].Steps) != 2 || got[0].Steps[1].ExitCode == nil || *got[0].Steps[1].ExitCode != 3 {
		t.Fatalf("jobs = %+v err %v", got, err)
	}

	if err := db.AppendPipelineLogs(ctx, []PipelineLogLine{{RunID: "r1", JobKey: "test", Line: "one", CreatedAt: now}, {RunID: "r1", JobKey: "test", Line: "two", CreatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	lines, _ := db.ListPipelineLogs(ctx, "r1", "", lines0(t, db), 10)
	if len(lines) != 1 || lines[0].Line != "two" {
		t.Fatalf("logs after first = %+v", lines)
	}

	active, _ := db.ListActivePipelineRuns(ctx)
	if len(active) != 2 {
		t.Fatalf("active = %d", len(active))
	}
	if err := db.RequestPipelineRunCancel(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if err := db.RequestPipelineRunCancel(ctx, "missing"); !errors.Is(err, ErrPipelineRunNotFound) {
		t.Fatalf("cancel missing err = %v", err)
	}
	if err := db.SetPipelineRunStatus(ctx, "r1", PipelineStatusSucceeded, "ok", &now, &now); err != nil {
		t.Fatal(err)
	}
	if n, err := db.PrunePipelineRuns(ctx, 0); err != nil || n != 1 {
		t.Fatalf("prune = %d err %v", n, err)
	}
}

func lines0(t *testing.T, db *DB) int64 {
	t.Helper()
	all, err := db.ListPipelineLogs(context.Background(), "r1", "", 0, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("all logs = %+v %v", all, err)
	}
	return all[0].ID
}

func TestPipelineApprovalDecidedOnce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	p, _ := db.SavePipeline(ctx, Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: "a", CreatedAt: now, UpdatedAt: now})
	_, _ = db.CreatePipelineRun(ctx, PipelineRun{ID: "r1", PipelineID: p.ID, AppName: "web", TriggerKind: "manual", Definition: "a", CreatedAt: now})
	_ = db.CreatePipelineJobs(ctx, []PipelineJob{{ID: "j1", RunID: "r1", Key: "deploy", DisplayName: "deploy", NeedsJSON: "[]", MatrixJSON: "{}", Status: PipelineStatusPending}})

	a := PipelineApproval{RunID: "r1", JobID: "j1", StepIndex: 0, RequiredAbility: "deploy", CreatedAt: now}
	if err := db.CreatePipelineApproval(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.CreatePipelineApproval(ctx, a); err != nil {
		t.Fatal(err)
	}
	list, _ := db.ListPipelineApprovals(ctx, "r1")
	if len(list) != 1 {
		t.Fatalf("approvals = %d, want 1", len(list))
	}
	if ok, _ := db.DecidePipelineApproval(ctx, list[0].ID, "approved", "u", "", now); !ok {
		t.Fatal("first decision should apply")
	}
	if ok, _ := db.DecidePipelineApproval(ctx, list[0].ID, "rejected", "u", "", now); ok {
		t.Fatal("second decision should not apply")
	}
}
