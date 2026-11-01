package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

const sinkCanary = "canary-store-sink-8841" //nolint:gosec // synthetic test value

func TestRecordScheduledTaskRunMasksOutput(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	secretmask.Add(sinkCanary)
	seedScheduledTaskService(t, db, "web")
	if err := db.SaveScheduledTask(ctx, newTestScheduledTask("st_mask", "web")); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordScheduledTaskRun(ctx, "st_mask", time.Now(), ScheduledTaskStatusFailed, "exit 1: host "+sinkCanary+" refused"); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetScheduledTask(ctx, "st_mask")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.LastRunOutput, sinkCanary) {
		t.Fatalf("last_run_output stores the secret: %q", got.LastRunOutput)
	}
}

func TestFinishDeployAttemptMasksError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	secretmask.Add(sinkCanary)
	if err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: "dep_mask", ServiceName: "web", Image: "x:1", Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_mask", DeployAttemptStatusFailed, time.Now(), "pull failed for "+sinkCanary); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(`SELECT error FROM deploy_attempts WHERE id = 'dep_mask'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, sinkCanary) {
		t.Fatalf("deploy_attempts.error stores the secret: %q", stored)
	}
}

func TestSetPipelineJobOutputsMasksValue(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	secretmask.Add(sinkCanary)
	now := time.Now().UTC()
	if _, err := db.SavePipeline(ctx, Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: "a", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreatePipelineRun(ctx, PipelineRun{ID: "r1", PipelineID: "p1", AppName: "web", TriggerKind: "manual", Definition: "a", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreatePipelineJobs(ctx, []PipelineJob{{ID: "j1", RunID: "r1", Key: "t", DisplayName: "t", NeedsJSON: "[]", MatrixJSON: "{}", Status: PipelineStatusPending}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPipelineJobOutputs(ctx, "j1", `{"url":"`+sinkCanary+`"}`); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(`SELECT outputs FROM pipeline_jobs WHERE id = 'j1'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, sinkCanary) {
		t.Fatalf("pipeline_jobs.outputs stores the secret: %q", stored)
	}
}
