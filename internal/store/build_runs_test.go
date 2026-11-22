package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestListBuildAttemptsPage_Keyset(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i := 1; i <= 3; i++ {
		a := DeployAttempt{ID: fmt.Sprintf("dep_%d", i), ServiceName: "web", Source: DeployAttemptSourceWebhook, Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Duration(i) * time.Minute)}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	first, err := db.ListBuildAttemptsPage(ctx, 2, "")
	if err != nil || len(first) != 2 || first[0].ID != "dep_3" {
		t.Fatalf("first = %v, %v", first, err)
	}
	rest, err := db.ListBuildAttemptsPage(ctx, 2, EncodeDeploymentCursor(first[1].StartedAt, first[1].ID))
	if err != nil || len(rest) != 1 || rest[0].ID != "dep_1" {
		t.Fatalf("rest = %v, %v", rest, err)
	}
}
