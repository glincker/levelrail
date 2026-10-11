package store

import (
	"context"
	"testing"
	"time"
)

// LatestDeployActivity must agree with ListDeployAttempts()[0] for every
// service, including the unfinished and never deployed cases.
func TestLatestDeployActivity_MatchesListDeployAttempts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Millisecond)

	for _, name := range []string{"finished", "running", "never"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:1", Port: 3000}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	fin := base.Add(-time.Minute)
	attempts := []DeployAttempt{
		{ID: "dep_old", ServiceName: "finished", Image: "i", Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(-time.Hour)},
		{ID: "dep_new", ServiceName: "finished", Image: "i", Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(-2 * time.Minute), FinishedAt: &fin},
		{ID: "dep_run", ServiceName: "running", Image: "i", Status: DeployAttemptStatusRunning, StartedAt: base.Add(-30 * time.Second)},
	}
	for _, a := range attempts {
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("save %s: %v", a.ID, err)
		}
	}

	got, err := db.LatestDeployActivity(ctx)
	if err != nil {
		t.Fatalf("LatestDeployActivity: %v", err)
	}
	if _, ok := got["never"]; ok {
		t.Error("service without attempts must be absent")
	}
	for _, name := range []string{"finished", "running"} {
		list, err := db.ListDeployAttempts(ctx, name)
		if err != nil || len(list) == 0 {
			t.Fatalf("list %s: %v", name, err)
		}
		want := list[0].StartedAt
		if list[0].FinishedAt != nil {
			want = *list[0].FinishedAt
		}
		if !got[name].Equal(want) {
			t.Errorf("%s = %v, want %v", name, got[name], want)
		}
	}
}
