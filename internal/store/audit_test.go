package store

import (
	"context"
	"testing"
	"time"
)

func testAuditEntry(id, createdAt string) AuditEntry {
	return AuditEntry{
		ID:         id,
		ActorType:  "session",
		ActorID:    "user_1",
		ActorName:  "admin@example.com",
		Ability:    "write",
		Method:     "POST",
		Path:       "/api/v1/apps",
		StatusCode: 201,
		RemoteAddr: "127.0.0.1",
		CreatedAt:  createdAt,
	}
}

func TestSaveAndListAuditEntries_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	older := testAuditEntry("aud_1", "2026-01-01T00:00:00.000000000Z")
	newer := testAuditEntry("aud_2", "2026-01-02T00:00:00.000000000Z")

	if err := db.SaveAuditEntry(ctx, older); err != nil {
		t.Fatalf("SaveAuditEntry(older) error = %v", err)
	}
	if err := db.SaveAuditEntry(ctx, newer); err != nil {
		t.Fatalf("SaveAuditEntry(newer) error = %v", err)
	}

	got, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != "aud_2" || got[1].ID != "aud_1" {
		t.Errorf("order = [%s %s], want [aud_2 aud_1] (newest first)", got[0].ID, got[1].ID)
	}
	if got[0].ActorName != "admin@example.com" || got[0].Ability != "write" {
		t.Errorf("got[0] = %+v, want actor_name/ability preserved", got[0])
	}
}

func TestListAuditEntries_Limit(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for i, ts := range []string{
		"2026-01-01T00:00:00.000000000Z",
		"2026-01-02T00:00:00.000000000Z",
		"2026-01-03T00:00:00.000000000Z",
	} {
		e := testAuditEntry("aud_lim_"+string(rune('a'+i)), ts)
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatalf("SaveAuditEntry(%d) error = %v", i, err)
		}
	}

	got, err := db.ListAuditEntries(ctx, 2, nil, AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (limit applied)", len(got))
	}
	if got[0].ID != "aud_lim_c" || got[1].ID != "aud_lim_b" {
		t.Errorf("order = [%s %s], want the two newest", got[0].ID, got[1].ID)
	}
}

func TestListAuditEntries_BeforeCursor(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := testAuditEntry("aud_c1", "2026-01-01T00:00:00.000000000Z")
	second := testAuditEntry("aud_c2", "2026-01-02T00:00:00.000000000Z")
	third := testAuditEntry("aud_c3", "2026-01-03T00:00:00.000000000Z")
	for _, e := range []AuditEntry{first, second, third} {
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatalf("SaveAuditEntry(%s) error = %v", e.ID, err)
		}
	}

	cursor, err := time.Parse(time.RFC3339Nano, third.CreatedAt)
	if err != nil {
		t.Fatalf("parse cursor: %v", err)
	}

	got, err := db.ListAuditEntries(ctx, 50, &cursor, AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries(before) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (third excluded by before cursor)", len(got))
	}
	if got[0].ID != "aud_c2" || got[1].ID != "aud_c1" {
		t.Errorf("order = [%s %s], want [aud_c2 aud_c1]", got[0].ID, got[1].ID)
	}
}

func TestListAuditEntries_Empty(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListAuditEntries(context.Background(), 50, nil, AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0 for an empty table", len(got))
	}
}

func TestListAuditEntries_FilterByPathAndMethod(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	appUpdate := testAuditEntry("aud_put", "2026-01-01T00:00:00.000000000Z")
	appUpdate.Method = "PUT"
	appUpdate.Path = "/api/v1/apps/web"

	appDelete := testAuditEntry("aud_delete", "2026-01-01T00:00:01.000000000Z")
	appDelete.Method = "DELETE"
	appDelete.Path = "/api/v1/apps/web"

	otherApp := testAuditEntry("aud_other", "2026-01-01T00:00:02.000000000Z")
	otherApp.Method = "PUT"
	otherApp.Path = "/api/v1/apps/other"

	for _, e := range []AuditEntry{appUpdate, appDelete, otherApp} {
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatalf("SaveAuditEntry(%s) error = %v", e.ID, err)
		}
	}

	got, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{Path: "/api/v1/apps/web", Method: "PUT"})
	if err != nil {
		t.Fatalf("ListAuditEntries(filter) error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "aud_put" {
		t.Fatalf("ListAuditEntries(filter) = %+v, want only aud_put", got)
	}
}
