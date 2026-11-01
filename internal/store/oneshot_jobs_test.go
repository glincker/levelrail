package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestOneShotJobs_RoundTripRecoverAndPrune(t *testing.T) {
	ctx := t.Context()
	db := openTestDB(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, st := range []string{"Succeeded", "Succeeded", "Running"} {
		j := OneShotJob{ID: "j" + string(rune('a'+i)), App: "web", TaskID: "t" + string(rune('a'+i)), State: st,
			Completions: 2, RunIDs: []string{"r1", "r2"}, StartedAt: base.Add(time.Duration(i) * time.Hour)}
		if err := db.SaveOneShotJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.GetOneShotJob(ctx, "jc")
	if err != nil || got.State != "Running" || len(got.RunIDs) != 2 || got.FinishedAt != nil {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := db.GetOneShotJob(ctx, "nope"); !errors.Is(err, ErrOneShotJobNotFound) {
		t.Fatalf("missing job err = %v", err)
	}

	n, err := db.FailInterruptedOneShotJobs(ctx, "restart", base.Add(24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("interrupted = %d, %v", n, err)
	}
	got, _ = db.GetOneShotJob(ctx, "jc")
	if got.State != "Failed" || got.Message != "restart" || got.FinishedAt == nil || got.Active != 0 {
		t.Fatalf("after recover = %+v", got)
	}
	if n, _ = db.FailInterruptedOneShotJobs(ctx, "restart", base); n != 0 {
		t.Fatalf("second recover touched %d rows", n)
	}

	tasks, err := db.PruneOneShotJobs(ctx, 2)
	if err != nil || len(tasks) != 1 || tasks[0] != "ta" {
		t.Fatalf("prune = %v, %v", tasks, err)
	}
	if _, err := db.GetOneShotJob(ctx, "ja"); !errors.Is(err, ErrOneShotJobNotFound) {
		t.Fatalf("oldest job should be pruned, err = %v", err)
	}
}

func TestOneShotJobs_MessageIsMasked(t *testing.T) {
	ctx := t.Context()
	db := openTestDB(t)
	secretmask.Add("oneshot-secret-value-1234")
	j := OneShotJob{ID: "jm", App: "web", TaskID: "tm", State: "Failed", Message: "boom oneshot-secret-value-1234 end", StartedAt: time.Now()}
	if err := db.SaveOneShotJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetOneShotJob(ctx, "jm")
	if err != nil || strings.Contains(got.Message, "oneshot-secret") || !strings.HasPrefix(got.Message, "boom ") || !strings.HasSuffix(got.Message, " end") {
		t.Fatalf("message = %q, %v", got.Message, err)
	}
}
