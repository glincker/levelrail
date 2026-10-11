package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openExportTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestExportDestinationLifecycle(t *testing.T) {
	db := openExportTestDB(t)
	ctx := context.Background()
	d := ExportDestination{
		ID: "exp_1", Name: "loki", Kind: ExportKindLogsLoki, Endpoint: "https://loki.example.com", Enabled: true,
		Apps: []string{"web"}, MinLevel: "warn", CreatedAt: "2026-10-11T00:00:00Z", UpdatedAt: "2026-10-11T00:00:00Z",
		Options: ExportOptions{Labels: map[string]string{"env": "prod"}, HeaderNames: []string{"X-Scope-OrgID"}},
	}
	if err := db.SaveExportDestination(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dup := d
	dup.ID = "exp_2"
	if err := db.SaveExportDestination(ctx, dup); !errors.Is(err, ErrExportDestinationNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}

	got, err := db.GetExportDestination(ctx, "exp_1")
	if err != nil || got.Name != "loki" || got.Apps[0] != "web" || got.Options.Labels["env"] != "prod" || !got.Enabled {
		t.Fatalf("Get = %+v err = %v", got, err)
	}

	got.Enabled = false
	got.UpdatedAt = "2026-10-11T01:00:00Z"
	if err := db.UpdateExportDestination(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if again, _ := db.GetExportDestination(ctx, "exp_1"); again.Enabled || again.UpdatedAt != got.UpdatedAt {
		t.Fatalf("update not persisted: %+v", again)
	}

	if err := db.SaveExportStatus(ctx, ExportStatus{DestinationID: "exp_1", SentTotal: 5, CursorAt: "a", CursorID: "b", CircuitOpen: true}); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}
	if err := db.SaveExportStatus(ctx, ExportStatus{DestinationID: "exp_1", SentTotal: 9, CursorAt: "c", CursorID: "d"}); err != nil {
		t.Fatalf("SaveStatus upsert: %v", err)
	}
	st, _ := db.GetExportStatus(ctx, "exp_1")
	if st.SentTotal != 9 || st.CursorID != "d" || st.CircuitOpen {
		t.Fatalf("status = %+v", st)
	}
	if empty, err := db.GetExportStatus(ctx, "exp_none"); err != nil || empty.SentTotal != 0 {
		t.Fatalf("missing status = %+v err = %v", empty, err)
	}

	if err := db.DeleteExportDestination(ctx, "exp_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.GetExportDestination(ctx, "exp_1"); !errors.Is(err, ErrExportDestinationNotFound) {
		t.Fatalf("Get after delete err = %v", err)
	}
	if st, _ := db.GetExportStatus(ctx, "exp_1"); st.SentTotal != 0 {
		t.Fatalf("status should cascade away, got %+v", st)
	}
	if err := db.DeleteExportDestination(ctx, "exp_1"); !errors.Is(err, ErrExportDestinationNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}

func TestListAuditEntriesAfterCursorOrderAndBounds(t *testing.T) {
	db := openExportTestDB(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	rows := []struct {
		id string
		at time.Time
	}{
		{"a", base}, {"b", base}, {"c", base}, {"d", base.Add(time.Second)}, {"e", base.Add(2 * time.Second)},
	}
	for _, r := range rows {
		if err := db.SaveAuditEntry(ctx, AuditEntry{ID: r.id, ActorType: "token", Method: "POST", Path: "/x", StatusCode: 200, CreatedAt: FormatAuditTime(r.at)}); err != nil {
			t.Fatalf("SaveAuditEntry: %v", err)
		}
	}
	ids := func(es []AuditEntry) string {
		s := ""
		for _, e := range es {
			s += e.ID
		}
		return s
	}

	tests := []struct {
		name    string
		afterAt string
		afterID string
		to      *time.Time
		limit   int
		wantIDs string
	}{
		{"from start", "", "", nil, 10, "abcde"},
		{"tie break within one timestamp", FormatAuditTime(base), "a", nil, 10, "bcde"},
		{"cursor resumes mid tie", FormatAuditTime(base), "b", nil, 2, "cd"},
		{"upper bound exclusive", "", "", ptr(base.Add(time.Second)), 10, "abc"},
		{"past the end", FormatAuditTime(base.Add(2 * time.Second)), "e", nil, 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ListAuditEntriesAfter(ctx, tt.afterAt, tt.afterID, tt.to, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			if ids(got) != tt.wantIDs {
				t.Fatalf("ids = %q, want %q", ids(got), tt.wantIDs)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
