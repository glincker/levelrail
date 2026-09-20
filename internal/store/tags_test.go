package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedTagService(t *testing.T, db *DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed service %q: %v", name, err)
	}
}

func newTestTag(id, name string) Tag {
	return Tag{ID: id, Name: name, CreatedAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)}
}

func TestSaveAndGetTag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, want); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}

	got, err := db.GetTag(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetTag() error = %v", err)
	}
	if got.ID != want.ID || got.Name != want.Name {
		t.Errorf("GetTag() = %+v, want %+v", got, want)
	}
}

func TestGetTag_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetTag(ctx, "tag_missing")
	if !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTag() error = %v, want ErrTagNotFound", err)
	}
}

func TestGetTagByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, want); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}

	got, err := db.GetTagByName(ctx, "production")
	if err != nil {
		t.Fatalf("GetTagByName() error = %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("GetTagByName() ID = %q, want %q", got.ID, want.ID)
	}
}

func TestGetTagByName_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetTagByName(ctx, "missing")
	if !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTagByName() error = %v, want ErrTagNotFound", err)
	}
}

func TestSaveTag_DuplicateNameRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveTag(ctx, newTestTag("tag_1", "production")); err != nil {
		t.Fatalf("SaveTag(tag_1) error = %v", err)
	}
	err := db.SaveTag(ctx, newTestTag("tag_2", "production"))
	if !errors.Is(err, ErrTagNameTaken) {
		t.Fatalf("SaveTag() with a duplicate name error = %v, want ErrTagNameTaken", err)
	}
}

func TestListTags(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"staging", "backend", "production"} {
		if err := db.SaveTag(ctx, newTestTag("tag_"+name, name)); err != nil {
			t.Fatalf("SaveTag(%q) error = %v", name, err)
		}
	}

	got, err := db.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if len(got) != 3 || got[0].Name != "backend" || got[1].Name != "production" || got[2].Name != "staging" {
		t.Fatalf("ListTags() = %+v, want alphabetical [backend, production, staging]", got)
	}
}

func TestDeleteTag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tag := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, tag); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}

	if err := db.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag() error = %v", err)
	}
	if _, err := db.GetTag(ctx, tag.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("GetTag() after delete error = %v, want ErrTagNotFound", err)
	}
}

func TestDeleteTag_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteTag(ctx, "tag_missing")
	if !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("DeleteTag() error = %v, want ErrTagNotFound", err)
	}
}

func TestDeleteTag_CascadesAppTags(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")

	tag := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, tag); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}
	if err := db.AttachAppTag(ctx, tag.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}

	if err := db.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag() error = %v", err)
	}

	got, err := db.ListTagsForApp(ctx, "web")
	if err != nil {
		t.Fatalf("ListTagsForApp() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListTagsForApp() after tag delete = %+v, want empty", got)
	}
}

func TestAttachAndDetachAppTag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")

	tag := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, tag); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}

	if err := db.AttachAppTag(ctx, tag.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}
	got, err := db.ListTagsForApp(ctx, "web")
	if err != nil {
		t.Fatalf("ListTagsForApp() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != tag.ID {
		t.Fatalf("ListTagsForApp() = %+v, want [%+v]", got, tag)
	}

	// Attaching again is idempotent, not a conflict.
	if err := db.AttachAppTag(ctx, tag.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() second call error = %v, want nil (idempotent)", err)
	}
	got, err = db.ListTagsForApp(ctx, "web")
	if err != nil {
		t.Fatalf("ListTagsForApp() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListTagsForApp() after re-attach = %+v, want still exactly one", got)
	}

	if err := db.DetachAppTag(ctx, tag.ID, "web"); err != nil {
		t.Fatalf("DetachAppTag() error = %v", err)
	}
	got, err = db.ListTagsForApp(ctx, "web")
	if err != nil {
		t.Fatalf("ListTagsForApp() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListTagsForApp() after detach = %+v, want empty", got)
	}
}

func TestDetachAppTag_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")

	tag := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, tag); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}

	err := db.DetachAppTag(ctx, tag.ID, "web")
	if !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("DetachAppTag() on a never-attached pair error = %v, want ErrTagNotFound", err)
	}
}

func TestListTagsForApps_Batch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")
	seedTagService(t, db, "worker")
	seedTagService(t, db, "untagged")

	prod := newTestTag("tag_prod", "production")
	staging := newTestTag("tag_staging", "staging")
	for _, tag := range []Tag{prod, staging} {
		if err := db.SaveTag(ctx, tag); err != nil {
			t.Fatalf("SaveTag(%q) error = %v", tag.Name, err)
		}
	}
	if err := db.AttachAppTag(ctx, prod.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}
	if err := db.AttachAppTag(ctx, prod.ID, "worker"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}
	if err := db.AttachAppTag(ctx, staging.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}

	got, err := db.ListTagsForApps(ctx, []string{"web", "worker", "untagged"})
	if err != nil {
		t.Fatalf("ListTagsForApps() error = %v", err)
	}
	if len(got["web"]) != 2 {
		t.Errorf("ListTagsForApps()[web] = %+v, want 2 tags", got["web"])
	}
	if len(got["worker"]) != 1 || got["worker"][0].ID != prod.ID {
		t.Errorf("ListTagsForApps()[worker] = %+v, want [%+v]", got["worker"], prod)
	}
	if _, ok := got["untagged"]; ok {
		t.Errorf("ListTagsForApps()[untagged] present = %+v, want no key for an untagged app", got["untagged"])
	}
}

func TestListTagsForApps_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.ListTagsForApps(ctx, nil)
	if err != nil {
		t.Fatalf("ListTagsForApps(nil) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListTagsForApps(nil) = %+v, want empty map", got)
	}
}

func TestListAppNamesByTag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")
	seedTagService(t, db, "worker")

	tag := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, tag); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}
	if err := db.AttachAppTag(ctx, tag.ID, "worker"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}
	if err := db.AttachAppTag(ctx, tag.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}

	got, err := db.ListAppNamesByTag(ctx, tag.ID)
	if err != nil {
		t.Fatalf("ListAppNamesByTag() error = %v", err)
	}
	if len(got) != 2 || got[0] != "web" || got[1] != "worker" {
		t.Fatalf("ListAppNamesByTag() = %+v, want alphabetical [web, worker]", got)
	}
}

func TestAttachAppTag_ServiceDeletedCascades(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")

	tag := newTestTag("tag_1", "production")
	if err := db.SaveTag(ctx, tag); err != nil {
		t.Fatalf("SaveTag() error = %v", err)
	}
	if err := db.AttachAppTag(ctx, tag.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	got, err := db.ListAppNamesByTag(ctx, tag.ID)
	if err != nil {
		t.Fatalf("ListAppNamesByTag() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListAppNamesByTag() after app delete = %+v, want empty (ON DELETE CASCADE)", got)
	}
}
