package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestResourceNotesRoundTripAndClear(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetResourceNotes(ctx, LabelKindApp, "web", ResourceNotes{Description: "Checkout", Notes: "# Runbook", UpdatedBy: "ops"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetResourceNotes(ctx, LabelKindDatabase, "web", ResourceNotes{Notes: "db notes"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetResourceNotes(ctx, LabelKindApp, "web")
	if err != nil || got.Description != "Checkout" || got.Notes != "# Runbook" || got.UpdatedBy != "ops" || got.UpdatedAt == "" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	descs, err := db.ListResourceDescriptions(ctx, LabelKindApp)
	if err != nil || len(descs) != 1 || descs["web"] != "Checkout" {
		t.Fatalf("descriptions = %v, %v", descs, err)
	}
	if err := db.SetResourceNotes(ctx, LabelKindApp, "web", ResourceNotes{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetResourceNotes(ctx, LabelKindApp, "web"); got != (ResourceNotes{}) {
		t.Fatalf("clear left %+v", got)
	}
	if got, _ := db.GetResourceNotes(ctx, LabelKindDatabase, "web"); got.Notes != "db notes" {
		t.Fatalf("kinds must not collide: %+v", got)
	}
}

func TestResourceNotesDroppedOnDeleteThenRecreate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}))
	must(db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}))
	must(db.SetResourceNotes(ctx, LabelKindApp, "web", ResourceNotes{Notes: "old app runbook"}))
	must(db.SetResourceNotes(ctx, LabelKindDatabase, "main", ResourceNotes{Notes: "old db runbook"}))
	// Saving again is an upsert and must not drop the notes.
	must(db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:2", Port: 80}))
	if got, _ := db.GetResourceNotes(ctx, LabelKindApp, "web"); got.Notes != "old app runbook" {
		t.Fatalf("update dropped notes: %+v", got)
	}

	must(db.DeleteDesiredService(ctx, "web"))
	must(db.DeleteDesiredDatabase(ctx, "main"))
	must(db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}))
	must(db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}))
	for _, kind := range []string{LabelKindApp, LabelKindDatabase} {
		name := map[string]string{LabelKindApp: "web", LabelKindDatabase: "main"}[kind]
		if got, _ := db.GetResourceNotes(ctx, kind, name); got != (ResourceNotes{}) {
			t.Fatalf("recreated %s %s shows old notes: %+v", kind, name, got)
		}
	}
}

func TestUpdateResourceNotesConcurrentFieldsBothSurvive(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for i := range 20 {
		name := fmt.Sprintf("app-%d", i)
		d, n := "desc", "notes"
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- db.UpdateResourceNotes(ctx, LabelKindApp, name, ResourceNotesUpdate{Description: &d})
		}()
		go func() {
			defer wg.Done()
			errs <- db.UpdateResourceNotes(ctx, LabelKindApp, name, ResourceNotesUpdate{Notes: &n})
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if got, _ := db.GetResourceNotes(ctx, LabelKindApp, name); got.Description != "desc" || got.Notes != "notes" {
			t.Fatalf("%s lost an update: %+v", name, got)
		}
	}
	empty := ""
	if err := db.UpdateResourceNotes(ctx, LabelKindApp, "app-0", ResourceNotesUpdate{Description: &empty, Notes: &empty}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetResourceNotes(ctx, LabelKindApp, "app-0"); got != (ResourceNotes{}) {
		t.Fatalf("clearing both fields kept %+v", got)
	}
}
