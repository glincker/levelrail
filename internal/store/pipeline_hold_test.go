package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPipelineRunHoldLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := db.SavePipeline(ctx, Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: "a", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	held, err := db.CreatePipelineRun(ctx, PipelineRun{ID: "held", PipelineID: "p1", AppName: "web", TriggerKind: "pull_request", Definition: "a",
		ConcurrencyGroup: "g", HoldState: HoldPending, HoldReason: "fork", CreatedAt: now})
	if err != nil || held.HoldState != HoldPending {
		t.Fatalf("create held: %+v %v", held, err)
	}
	if _, err := db.CreatePipelineRun(ctx, PipelineRun{ID: "normal", PipelineID: "p1", AppName: "web", TriggerKind: "push", Definition: "a",
		ConcurrencyGroup: "g", CreatedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}

	group, _ := db.ListPipelineRunsInGroup(ctx, "g")
	if len(group) != 1 || group[0].ID != "normal" {
		t.Fatalf("a pending hold must not sit in the concurrency group: %+v", group)
	}

	ok, err := db.DecidePipelineRunHold(ctx, "held", true, "gagan", now)
	if err != nil || !ok {
		t.Fatalf("approve: ok=%v err=%v", ok, err)
	}
	if ok, _ := db.DecidePipelineRunHold(ctx, "held", false, "other", now); ok {
		t.Fatal("second decision accepted")
	}
	got, _ := db.GetPipelineRun(ctx, "held")
	if got.HoldState != HoldApproved || got.HoldBy != "gagan" || got.HoldAt == nil || got.HoldReason != "fork" {
		t.Fatalf("after approve: %+v", got)
	}
	if group, _ = db.ListPipelineRunsInGroup(ctx, "g"); len(group) != 2 {
		t.Fatalf("an approved run joins the group: %+v", group)
	}
}

func TestPipelineTriggerLogPrunesPerApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := range PipelineTriggerLogKeep + 10 {
		if err := db.AddPipelineTriggerLog(ctx, PipelineTriggerLog{AppName: "web", Event: "push", Decision: TriggerSkipped, Reason: fmt.Sprint(i), CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddPipelineTriggerLog(ctx, PipelineTriggerLog{AppName: "api", Event: "push", Decision: TriggerStarted, Reason: "ok", RunID: "r1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	web, err := db.ListPipelineTriggerLog(ctx, "web", 1000)
	if err != nil || len(web) != PipelineTriggerLogKeep {
		t.Fatalf("web log = %d err %v, want %d", len(web), err, PipelineTriggerLogKeep)
	}
	if want := fmt.Sprint(PipelineTriggerLogKeep + 9); web[0].Reason != want {
		t.Fatalf("newest first: got %q want %q", web[0].Reason, want)
	}
	api, _ := db.ListPipelineTriggerLog(ctx, "api", 0)
	if len(api) != 1 || api[0].RunID != "r1" {
		t.Fatalf("api log = %+v", api)
	}
}
