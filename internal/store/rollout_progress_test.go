package store

import (
	"context"
	"testing"
)

func TestDeleteDesiredServiceDropsRolloutProgress(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "web:1", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService: %v", err)
	}
	if err := db.SaveRolloutProgress(ctx, "web", []byte(`{"wave":1}`)); err != nil {
		t.Fatalf("SaveRolloutProgress: %v", err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService: %v", err)
	}
	if _, found, err := db.LoadRolloutProgress(ctx, "web"); err != nil || found {
		t.Fatalf("LoadRolloutProgress after delete = found %v, err %v; want none", found, err)
	}
	if err := db.DeleteRolloutProgress(ctx, "web"); err != nil {
		t.Fatalf("DeleteRolloutProgress on absent row: %v", err)
	}
}
