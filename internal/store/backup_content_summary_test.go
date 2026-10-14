package store

import (
	"context"
	"testing"
)

func TestBackupContentSummary_RoundTripAndCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_sum", DatabaseName: "search", TargetID: target.ID,
		ObjectKey: "search/search-1.dump", StartedAt: "2026-10-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	if got, err := db.GetBackupContentSummary(ctx, "bkh_sum"); err != nil || got != "" {
		t.Fatalf("GetBackupContentSummary() before set = %q, %v; want empty", got, err)
	}
	if err := db.SetBackupContentSummary(ctx, "bkh_sum", `{"objects":1}`); err != nil {
		t.Fatalf("SetBackupContentSummary() error = %v", err)
	}
	if err := db.SetBackupContentSummary(ctx, "bkh_sum", `{"objects":2}`); err != nil {
		t.Fatalf("SetBackupContentSummary() overwrite error = %v", err)
	}
	if got, err := db.GetBackupContentSummary(ctx, "bkh_sum"); err != nil || got != `{"objects":2}` {
		t.Fatalf("GetBackupContentSummary() = %q, %v", got, err)
	}

	if err := db.DeleteBackupHistory(ctx, "bkh_sum"); err != nil {
		t.Fatalf("DeleteBackupHistory() with a summary row must cascade: %v", err)
	}
	if got, err := db.GetBackupContentSummary(ctx, "bkh_sum"); err != nil || got != "" {
		t.Fatalf("summary after delete = %q, %v; want gone", got, err)
	}
}
