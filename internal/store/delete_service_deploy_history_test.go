package store

import (
	"context"
	"testing"
	"time"
)

func TestDeleteDesiredServiceClearsDeployHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, name := range []string{"web", "other"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: name + ":1", Port: 80}); err != nil {
			t.Fatalf("seed app: %v", err)
		}
		err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: "dep_" + name, ServiceName: name, Image: name + ":1", Status: DeployAttemptStatusSucceeded, StartedAt: now})
		if err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got, err := db.ListDeployAttempts(ctx, "web"); err != nil || len(got) != 0 {
		t.Fatalf("attempts after delete = %+v, %v; want none", got, err)
	}
	if got, err := db.ListDeployAttempts(ctx, "other"); err != nil || len(got) != 1 {
		t.Fatalf("other app attempts = %+v, %v; want 1", got, err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:alpine", Port: 80}); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if got, err := db.ListDeployAttempts(ctx, "web"); err != nil || len(got) != 0 {
		t.Fatalf("recreated app history = %+v, %v; want empty", got, err)
	}
}
