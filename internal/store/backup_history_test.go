package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedBackupTarget(t *testing.T, db *DB) BackupTarget {
	t.Helper()
	target := newTestBackupTarget()
	if err := db.SaveBackupTarget(context.Background(), target); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}
	return target
}

func TestStartAndFinishBackupHistory_Succeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	got := mustListOne(t, db, "mydb")
	if got.Status != BackupStatusRunning {
		t.Fatalf("status after Start = %q, want %q", got.Status, BackupStatusRunning)
	}

	if err := db.FinishBackupHistory(ctx, "bkh_1", BackupStatusSucceeded, 4096, "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}

	got = mustListOne(t, db, "mydb")
	if got.Status != BackupStatusSucceeded {
		t.Errorf("status = %q, want %q", got.Status, BackupStatusSucceeded)
	}
	if got.SizeBytes != 4096 {
		t.Errorf("SizeBytes = %d, want 4096", got.SizeBytes)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty on success", got.Error)
	}
	if got.FinishedAt != "2026-08-14T00:01:00Z" {
		t.Errorf("FinishedAt = %q, want the value passed to Finish", got.FinishedAt)
	}
}

func TestFinishBackupHistory_Failed_RecordsError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	if err := db.FinishBackupHistory(ctx, "bkh_1", BackupStatusFailed, 512, "upload: access denied", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}

	got := mustListOne(t, db, "mydb")
	if got.Status != BackupStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, BackupStatusFailed)
	}
	if got.Error != "upload: access denied" {
		t.Errorf("Error = %q, want the failure reason", got.Error)
	}
}

func TestFinishBackupHistory_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.FinishBackupHistory(ctx, "bkh_missing", BackupStatusSucceeded, 0, "", "2026-08-14T00:01:00Z")
	if !errors.Is(err, ErrBackupHistoryNotFound) {
		t.Fatalf("FinishBackupHistory() error = %v, want ErrBackupHistoryNotFound", err)
	}
}

func TestListBackupHistory_NewestFirst_ScopedToDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	for _, h := range []BackupHistory{
		{ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID, ObjectKey: "k1", StartedAt: "2026-08-14T00:00:00Z"},
		{ID: "bkh_2", DatabaseName: "mydb", TargetID: target.ID, ObjectKey: "k2", StartedAt: "2026-08-14T00:01:00Z"},
		{ID: "bkh_3", DatabaseName: "otherdb", TargetID: target.ID, ObjectKey: "k3", StartedAt: "2026-08-14T00:02:00Z"},
	} {
		if err := db.StartBackupHistory(ctx, h); err != nil {
			t.Fatalf("StartBackupHistory(%s) error = %v", h.ID, err)
		}
	}

	got, err := db.ListBackupHistory(ctx, "mydb", 50, nil)
	if err != nil {
		t.Fatalf("ListBackupHistory() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "bkh_2" || got[1].ID != "bkh_1" {
		t.Fatalf("ListBackupHistory(mydb) = %+v, want [bkh_2, bkh_1] newest first, otherdb excluded", got)
	}
}

// TestPruneBackupHistory_KeepsNewestSucceeded is the direct exercise of
// this feature's own retention scope (see PruneBackupHistory's own doc
// comment for why this is deliberately narrow): five succeeded backups,
// keep 2, expect the two newest survive and the three oldest are gone.
func TestPruneBackupHistory_KeepsNewestSucceeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	seedSucceeded := func(id, startedAt string) {
		t.Helper()
		if err := db.StartBackupHistory(ctx, BackupHistory{
			ID: id, DatabaseName: "mydb", TargetID: target.ID,
			ObjectKey: id, StartedAt: startedAt,
		}); err != nil {
			t.Fatalf("StartBackupHistory(%s) error = %v", id, err)
		}
		if err := db.FinishBackupHistory(ctx, id, BackupStatusSucceeded, 1, "", startedAt); err != nil {
			t.Fatalf("FinishBackupHistory(%s) error = %v", id, err)
		}
	}

	seedSucceeded("bkh_1", "2026-08-14T00:00:00Z")
	seedSucceeded("bkh_2", "2026-08-14T00:01:00Z")
	seedSucceeded("bkh_3", "2026-08-14T00:02:00Z")
	seedSucceeded("bkh_4", "2026-08-14T00:03:00Z")
	seedSucceeded("bkh_5", "2026-08-14T00:04:00Z")

	pruned, err := db.PruneBackupHistory(ctx, "mydb", 2, time.Time{})
	if err != nil {
		t.Fatalf("PruneBackupHistory() error = %v", err)
	}
	if len(pruned) != 3 {
		t.Errorf("pruned = %d, want 3", len(pruned))
	}

	got, err := db.ListBackupHistory(ctx, "mydb", 50, nil)
	if err != nil {
		t.Fatalf("ListBackupHistory() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "bkh_5" || got[1].ID != "bkh_4" {
		t.Fatalf("ListBackupHistory(mydb) after prune = %+v, want [bkh_5, bkh_4] (2 newest)", got)
	}
}

// TestPruneBackupHistory_ReturnsPrunedTargetAndObjectKey proves the new
// return shape carries the actually-deleted rows' TargetID/ObjectKey,
// not just a count: internal/backup.Scheduler needs both to delete the
// matching object from the target bucket after a store-level prune.
func TestPruneBackupHistory_ReturnsPrunedTargetAndObjectKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	seedSucceeded := func(id, objectKey, startedAt string) {
		t.Helper()
		if err := db.StartBackupHistory(ctx, BackupHistory{
			ID: id, DatabaseName: "mydb", TargetID: target.ID,
			ObjectKey: objectKey, StartedAt: startedAt,
		}); err != nil {
			t.Fatalf("StartBackupHistory(%s) error = %v", id, err)
		}
		if err := db.FinishBackupHistory(ctx, id, BackupStatusSucceeded, 1, "", startedAt); err != nil {
			t.Fatalf("FinishBackupHistory(%s) error = %v", id, err)
		}
	}

	seedSucceeded("bkh_1", "mydb/mydb-1.dump", "2026-08-14T00:00:00Z")
	seedSucceeded("bkh_2", "mydb/mydb-2.dump", "2026-08-14T00:01:00Z")

	pruned, err := db.PruneBackupHistory(ctx, "mydb", 1, time.Time{})
	if err != nil {
		t.Fatalf("PruneBackupHistory() error = %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("pruned = %+v, want 1 row", pruned)
	}
	if pruned[0].TargetID != target.ID || pruned[0].ObjectKey != "mydb/mydb-1.dump" {
		t.Errorf("pruned[0] = %+v, want TargetID=%q ObjectKey=%q", pruned[0], target.ID, "mydb/mydb-1.dump")
	}
}

// TestPruneBackupHistory_NeverTouchesFailedOrRunning: a retain count
// bounds successful backups only, per PruneBackupHistory's own doc
// comment; a failed attempt and one still in flight both keep their
// diagnostic value regardless of how many successful backups exist.
func TestPruneBackupHistory_NeverTouchesFailedOrRunning(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	seed := func(id, status, startedAt string) {
		t.Helper()
		if err := db.StartBackupHistory(ctx, BackupHistory{
			ID: id, DatabaseName: "mydb", TargetID: target.ID,
			ObjectKey: id, StartedAt: startedAt,
		}); err != nil {
			t.Fatalf("StartBackupHistory(%s) error = %v", id, err)
		}
		if status != BackupStatusRunning {
			if err := db.FinishBackupHistory(ctx, id, status, 1, "", startedAt); err != nil {
				t.Fatalf("FinishBackupHistory(%s) error = %v", id, err)
			}
		}
	}

	seed("bkh_ok1", BackupStatusSucceeded, "2026-08-14T00:00:00Z")
	seed("bkh_ok2", BackupStatusSucceeded, "2026-08-14T00:01:00Z")
	seed("bkh_failed", BackupStatusFailed, "2026-08-14T00:02:00Z")
	seed("bkh_running", BackupStatusRunning, "2026-08-14T00:03:00Z")

	pruned, err := db.PruneBackupHistory(ctx, "mydb", 1, time.Time{})
	if err != nil {
		t.Fatalf("PruneBackupHistory() error = %v", err)
	}
	if len(pruned) != 1 {
		t.Errorf("pruned = %d, want 1 (only the older succeeded row)", len(pruned))
	}

	got, err := db.ListBackupHistory(ctx, "mydb", 50, nil)
	if err != nil {
		t.Fatalf("ListBackupHistory() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListBackupHistory(mydb) after prune = %+v, want 3 rows (failed + running + 1 succeeded)", got)
	}
	for _, h := range got {
		if h.ID == "bkh_ok1" {
			t.Errorf("bkh_ok1 (older succeeded) should have been pruned, still present: %+v", h)
		}
	}
}

// TestPruneBackupHistory_ZeroOrNegativeKeepDeletesNothing: BackupRetain
// 0 means "keep everything," the meaning migrations/0023's own doc
// comment establishes for a database that never opted into retention.
func TestPruneBackupHistory_ZeroOrNegativeKeepDeletesNothing(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "k1", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}
	if err := db.FinishBackupHistory(ctx, "bkh_1", BackupStatusSucceeded, 1, "", "2026-08-14T00:00:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}

	for _, keep := range []int{0, -1} {
		pruned, err := db.PruneBackupHistory(ctx, "mydb", keep, time.Time{})
		if err != nil {
			t.Fatalf("PruneBackupHistory(keep=%d) error = %v", keep, err)
		}
		if len(pruned) != 0 {
			t.Errorf("PruneBackupHistory(keep=%d) pruned = %d, want 0", keep, len(pruned))
		}
	}

	got, err := db.ListBackupHistory(ctx, "mydb", 50, nil)
	if err != nil {
		t.Fatalf("ListBackupHistory() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListBackupHistory(mydb) = %+v, want the 1 seeded row untouched", got)
	}
}

// TestPruneBackupHistory_RetentionDimensions is table-driven over the
// count and age retention dimensions: count only, age only, both set
// together (a row can violate one, the other, both, or neither), and
// neither set. Five succeeded backups spaced one day apart, dates
// 2026-08-10 through 2026-08-14.
func TestPruneBackupHistory_RetentionDimensions(t *testing.T) {
	dates := []string{
		"2026-08-10T00:00:00Z",
		"2026-08-11T00:00:00Z",
		"2026-08-12T00:00:00Z",
		"2026-08-13T00:00:00Z",
		"2026-08-14T00:00:00Z",
	}
	ids := []string{"bkh_1", "bkh_2", "bkh_3", "bkh_4", "bkh_5"}

	tests := []struct {
		name       string
		keep       int
		olderThan  time.Time
		wantRemain []string
	}{
		{
			name:       "neither set keeps everything",
			keep:       0,
			olderThan:  time.Time{},
			wantRemain: ids,
		},
		{
			name:       "count only keeps newest N",
			keep:       2,
			olderThan:  time.Time{},
			wantRemain: []string{"bkh_4", "bkh_5"},
		},
		{
			name:       "age only keeps everything at or after the cutoff",
			keep:       0,
			olderThan:  time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
			wantRemain: []string{"bkh_3", "bkh_4", "bkh_5"},
		},
		{
			// keep=4 alone would keep bkh_2..bkh_5; the age cutoff then
			// also removes bkh_2 and bkh_3 despite both surviving the
			// count limit, proving a row is pruned if it violates
			// *either* limit, not only the one that already caught it.
			name:       "both set prunes anything violating either limit",
			keep:       4,
			olderThan:  time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
			wantRemain: []string{"bkh_4", "bkh_5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			target := seedBackupTarget(t, db)

			for i, id := range ids {
				if err := db.StartBackupHistory(ctx, BackupHistory{
					ID: id, DatabaseName: "mydb", TargetID: target.ID,
					ObjectKey: id, StartedAt: dates[i],
				}); err != nil {
					t.Fatalf("StartBackupHistory(%s) error = %v", id, err)
				}
				if err := db.FinishBackupHistory(ctx, id, BackupStatusSucceeded, 1, "", dates[i]); err != nil {
					t.Fatalf("FinishBackupHistory(%s) error = %v", id, err)
				}
			}

			if _, err := db.PruneBackupHistory(ctx, "mydb", tt.keep, tt.olderThan); err != nil {
				t.Fatalf("PruneBackupHistory() error = %v", err)
			}

			got, err := db.ListBackupHistory(ctx, "mydb", 50, nil)
			if err != nil {
				t.Fatalf("ListBackupHistory() error = %v", err)
			}
			gotIDs := make(map[string]bool, len(got))
			for _, h := range got {
				gotIDs[h.ID] = true
			}
			if len(got) != len(tt.wantRemain) {
				t.Fatalf("remaining rows = %+v, want %v", got, tt.wantRemain)
			}
			for _, id := range tt.wantRemain {
				if !gotIDs[id] {
					t.Errorf("%s should have survived pruning, remaining = %+v", id, got)
				}
			}
		})
	}
}

// TestListBackupHistory_Pagination is table-driven over limit/before
// combinations against one fixed, deterministic seed of five rows, the
// same "seed once, assert many shapes" structure
// TestPruneBackupHistory_NeverTouchesFailedOrRunning already uses in this
// file.
func TestListBackupHistory_Pagination(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	for i, id := range []string{"bkh_1", "bkh_2", "bkh_3", "bkh_4", "bkh_5"} {
		startedAt := time.Date(2026, 1, 1, i, 0, 0, 0, time.UTC).Format(time.RFC3339)
		if err := db.StartBackupHistory(ctx, BackupHistory{
			ID: id, DatabaseName: "mydb", TargetID: target.ID,
			ObjectKey: id, StartedAt: startedAt,
		}); err != nil {
			t.Fatalf("StartBackupHistory(%s) error = %v", id, err)
		}
	}

	before3 := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		limit  int
		before *time.Time
		want   []string
	}{
		{name: "default limit returns newest first", limit: 50, before: nil, want: []string{"bkh_5", "bkh_4", "bkh_3", "bkh_2", "bkh_1"}},
		{name: "limit caps the page", limit: 2, before: nil, want: []string{"bkh_5", "bkh_4"}},
		{name: "before excludes rows at or after cursor", limit: 50, before: &before3, want: []string{"bkh_2", "bkh_1"}},
		{name: "limit and before combine", limit: 1, before: &before3, want: []string{"bkh_2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ListBackupHistory(ctx, "mydb", tt.limit, tt.before)
			if err != nil {
				t.Fatalf("ListBackupHistory() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ListBackupHistory() = %d rows, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, id := range tt.want {
				if got[i].ID != id {
					t.Errorf("row %d ID = %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}

func mustListOne(t *testing.T, db *DB, databaseName string) BackupHistory {
	t.Helper()
	got, err := db.ListBackupHistory(context.Background(), databaseName, 50, nil)
	if err != nil {
		t.Fatalf("ListBackupHistory(%q) error = %v", databaseName, err)
	}
	if len(got) != 1 {
		t.Fatalf("ListBackupHistory(%q) = %+v, want exactly 1 row", databaseName, got)
	}
	return got[0]
}
