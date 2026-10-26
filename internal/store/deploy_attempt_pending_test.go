package store

import (
	"context"
	"testing"
	"time"
)

func TestResolvePendingDeployAttempts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)
	saveAttempt(t, db, "dep_old", DeployAttemptStatusPending, base)
	saveAttempt(t, db, "dep_new", DeployAttemptStatusPending, base.Add(time.Minute))
	saveAttempt(t, db, "dep_done", DeployAttemptStatusSucceeded, base.Add(2*time.Minute))

	n, err := db.ResolvePendingDeployAttempts(ctx, "web", time.Now())
	if err != nil || n != 2 {
		t.Fatalf("resolve = %d, %v", n, err)
	}
	old, _ := db.GetDeployAttempt(ctx, "dep_old")
	if old.Status != DeployAttemptStatusSuperseded || old.SupersededBy != "dep_new" || old.FinishedAt == nil {
		t.Fatalf("older pending = %+v", old)
	}
	newest, _ := db.GetDeployAttempt(ctx, "dep_new")
	if newest.Status != DeployAttemptStatusSucceeded || newest.FinishedAt == nil {
		t.Fatalf("newest pending = %+v", newest)
	}
	if n, _ := db.ResolvePendingDeployAttempts(ctx, "web", time.Now()); n != 0 {
		t.Fatalf("second resolve moved %d rows", n)
	}
}
