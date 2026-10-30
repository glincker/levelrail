package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestInternalIncidents_UpgradeAndPublicFilter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3060))
	if err != nil {
		t.Fatalf("open at 3060: %v", err)
	}
	if _, err := old.ExecContext(ctx, `INSERT INTO status_incidents (id, kind, title, status, impact, component_ids, starts_at, created_at, updated_at)
		VALUES ('old1', 'incident', 'Old', 'resolved', 'minor', '[]', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = db.Close() }()

	got, err := db.GetInternalStatusIncident(ctx, "old1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Published || got.Source != "manual" || got.InternalState != IncidentStateResolved {
		t.Errorf("existing row = %+v, want published manual resolved", got.StatusIncidentInternal)
	}

	now := time.Now().UTC()
	sys := InternalStatusIncident{
		StatusIncident:         StatusIncident{ID: "sys1", Kind: "incident", Title: "Monitor down", Status: "investigating", Impact: "major", StartsAt: now},
		StatusIncidentInternal: StatusIncidentInternal{Source: "monitor", SourceRef: "mon1", InternalState: IncidentStateTriggered},
	}
	if err := db.CreateInternalStatusIncident(ctx, sys); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInternalIncidentUpdate(ctx, StatusIncidentUpdate{ID: "u1", IncidentID: "sys1", Status: "investigating", Body: "probe said x", CreatedAt: now}, UpdateVisibilityInternal, "system"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInternalIncidentUpdate(ctx, StatusIncidentUpdate{ID: "u2", IncidentID: "old1", Status: "resolved", Body: "fixed", CreatedAt: now}, UpdateVisibilityInternal, "op"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddStatusIncidentUpdate(ctx, StatusIncidentUpdate{ID: "u3", IncidentID: "old1", Status: "resolved", Body: "public note", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	pub, err := db.ListPublicStatusIncidents(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != 1 || pub[0].ID != "old1" {
		t.Errorf("public list = %+v, want only old1", pub)
	}
	if _, err := db.GetPublicStatusIncident(ctx, "sys1"); !errors.Is(err, ErrStatusIncidentNotFound) {
		t.Errorf("public get of unpublished = %v, want not found", err)
	}
	ups, err := db.ListPublicStatusIncidentUpdates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 1 || ups[0].ID != "u3" {
		t.Errorf("public updates = %+v, want only u3", ups)
	}
	if all, _ := db.ListInternalStatusIncidents(ctx, 50); len(all) != 2 {
		t.Errorf("operator list = %d rows, want 2", len(all))
	}

	dup := sys
	dup.ID = "sys2"
	if err := db.CreateInternalStatusIncident(ctx, dup); err == nil {
		t.Error("second open incident for one monitor must be rejected")
	}
	if err := db.SetStatusIncidentInternalState(ctx, "sys1", IncidentStateResolved, "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInternalStatusIncident(ctx, dup); err != nil {
		t.Errorf("new incident after resolve: %v", err)
	}
}
