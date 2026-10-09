package store

import (
	"errors"
	"testing"
	"time"
)

func TestDatabaseDataImportLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	t0 := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	stale := time.Hour

	if _, found, err := db.GetDatabaseDataImport(ctx, "main"); err != nil || found {
		t.Fatalf("missing row = %v, %v", found, err)
	}
	if err := db.ClaimDatabaseDataImport(ctx, "main", "old.example.com", 5432, "app", t0, stale); err != nil {
		t.Fatal(err)
	}
	err := db.ClaimDatabaseDataImport(ctx, "main", "old.example.com", 5432, "app", t0.Add(time.Minute), stale)
	if !errors.Is(err, ErrDataImportInProgress) {
		t.Fatalf("second claim = %v, want ErrDataImportInProgress", err)
	}
	if err := db.FinishDatabaseDataImport(ctx, "main", DataImportFailed, "boom", 2, 1, `{"checked":2}`, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	row, found, err := db.GetDatabaseDataImport(ctx, "main")
	if err != nil || !found || row.Status != DataImportFailed || row.Reason != "boom" || row.Mismatched != 1 || row.SourceHost != "old.example.com" {
		t.Fatalf("row = %+v, %v, %v", row, found, err)
	}

	if err := db.ClaimDatabaseDataImport(ctx, "main", "new.example.com", 5433, "app2", t0.Add(3*time.Minute), stale); err != nil {
		t.Fatalf("re-claim after a finished run = %v", err)
	}
	row, _, _ = db.GetDatabaseDataImport(ctx, "main")
	if row.Status != DataImportCopying || row.Reason != "" || row.SourceHost != "new.example.com" || row.Checked != 0 {
		t.Fatalf("re-claimed row = %+v", row)
	}

	if err := db.ClaimDatabaseDataImport(ctx, "main", "h", 1, "d", t0.Add(3*time.Minute+stale+time.Second), stale); err != nil {
		t.Fatalf("claim over a stale copying row = %v", err)
	}
	list, err := db.ListDatabaseDataImports(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
