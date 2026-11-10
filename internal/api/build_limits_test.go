package api

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBuildLimit_ThirdBuildWaitsUntilOneFinishes(t *testing.T) {
	t.Setenv("APP_DEPLOY_MAX_CONCURRENT_PER_PROJECT", "2")
	rt, db, cookie, gb := newQueueRouter(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "P", CreatedAt: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"api", "worker"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "levelrail/" + n + ":1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"web", "api", "worker"} {
		if err := db.UpdateServiceProject(ctx, n, "proj_1"); err != nil {
			t.Fatal(err)
		}
	}
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	second := postBuild(t, rt, cookie, "api", "main")
	gb.awaitStart(t)
	third := postBuild(t, rt, cookie, "worker", "main")
	if third.Status != store.DeployAttemptStatusQueued {
		t.Fatalf("third build = %+v, want queued", third)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, first.ID, store.DeployAttemptStatusSucceeded)
	if got := gb.awaitStart(t); got != third.ID {
		t.Fatalf("started %s, want %s", got, third.ID)
	}
	gb.release <- struct{}{}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, second.ID, store.DeployAttemptStatusSucceeded)
	awaitAttemptStatus(t, db, third.ID, store.DeployAttemptStatusSucceeded)
}
