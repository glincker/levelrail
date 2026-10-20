package store

import (
	"context"
	"testing"
	"time"
)

func TestUpsertAndGetHookRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	ranAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	run := HookRun{
		ServiceName: "web",
		HookType:    HookTypePreDeploy,
		Command:     "rails db:migrate",
		ExitCode:    0,
		Success:     true,
		Output:      "migrated\n",
		RanAt:       ranAt,
	}
	if err := db.UpsertHookRun(ctx, run); err != nil {
		t.Fatalf("UpsertHookRun() error = %v", err)
	}

	got, err := db.GetHookRuns(ctx, "web")
	if err != nil {
		t.Fatalf("GetHookRuns() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetHookRuns() = %d rows, want 1", len(got))
	}
	if got[0].ServiceName != run.ServiceName || got[0].HookType != run.HookType || got[0].Command != run.Command ||
		got[0].ExitCode != run.ExitCode || got[0].Success != run.Success || got[0].Output != run.Output ||
		!got[0].RanAt.Equal(run.RanAt) {
		t.Errorf("GetHookRuns() = %+v, want %+v", got[0], run)
	}
}

// TestUpsertHookRun_ReplacesPreviousRun asserts migrations/0082's own
// comment: only the most recent run per (service, hook_type) is kept,
// not a growing history.
func TestUpsertHookRun_ReplacesPreviousRun(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := HookRun{ServiceName: "web", HookType: HookTypePreDeploy, Command: "migrate v1", ExitCode: 1, Success: false, Output: "failed", RanAt: time.Now().UTC()}
	second := HookRun{ServiceName: "web", HookType: HookTypePreDeploy, Command: "migrate v2", ExitCode: 0, Success: true, Output: "ok", RanAt: time.Now().UTC().Add(time.Minute)}

	if err := db.UpsertHookRun(ctx, first); err != nil {
		t.Fatalf("UpsertHookRun(first) error = %v", err)
	}
	if err := db.UpsertHookRun(ctx, second); err != nil {
		t.Fatalf("UpsertHookRun(second) error = %v", err)
	}

	got, err := db.GetHookRuns(ctx, "web")
	if err != nil {
		t.Fatalf("GetHookRuns() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetHookRuns() = %d rows, want 1 (upsert must replace, not accumulate)", len(got))
	}
	if got[0].Command != "migrate v2" || !got[0].Success {
		t.Errorf("GetHookRuns() = %+v, want the second run to have replaced the first", got[0])
	}
}

// TestGetHookRuns_BothTypesIndependent asserts pre_deploy and post_deploy
// are tracked independently (PRIMARY KEY (service_name, hook_type)): a
// service with both configured gets two rows, not one overwriting the
// other.
func TestGetHookRuns_BothTypesIndependent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	pre := HookRun{ServiceName: "web", HookType: HookTypePreDeploy, Command: "migrate", ExitCode: 0, Success: true, Output: "ok", RanAt: time.Now().UTC()}
	post := HookRun{ServiceName: "web", HookType: HookTypePostDeploy, Command: "notify", ExitCode: 0, Success: true, Output: "sent", RanAt: time.Now().UTC()}
	if err := db.UpsertHookRun(ctx, pre); err != nil {
		t.Fatalf("UpsertHookRun(pre) error = %v", err)
	}
	if err := db.UpsertHookRun(ctx, post); err != nil {
		t.Fatalf("UpsertHookRun(post) error = %v", err)
	}

	got, err := db.GetHookRuns(ctx, "web")
	if err != nil {
		t.Fatalf("GetHookRuns() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetHookRuns() = %d rows, want 2", len(got))
	}
}

func TestGetHookRuns_NoneRecorded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetHookRuns(ctx, "web")
	if err != nil {
		t.Fatalf("GetHookRuns() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetHookRuns() = %+v, want none", got)
	}
}
