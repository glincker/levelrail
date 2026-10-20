package store

import (
	"context"
	"testing"
)

func TestPreviewAppSettings_DefaultsThenRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetPreviewAppSettings(ctx, "web")
	if err != nil {
		t.Fatalf("GetPreviewAppSettings() error = %v", err)
	}
	if got.OnLimit != PreviewOnLimitEvictOldest || got.AllowForkPreviews || got.TTLHours != 0 {
		t.Errorf("defaults = %+v", got)
	}

	if err := db.SavePreviewAppSettings(ctx, PreviewAppSettings{AppName: "web", OnLimit: PreviewOnLimitReject, AllowForkPreviews: true, TTLHours: 12}); err != nil {
		t.Fatalf("SavePreviewAppSettings() error = %v", err)
	}
	got, err = db.GetPreviewAppSettings(ctx, "web")
	if err != nil {
		t.Fatalf("GetPreviewAppSettings() error = %v", err)
	}
	if got.OnLimit != PreviewOnLimitReject || !got.AllowForkPreviews || got.TTLHours != 12 {
		t.Errorf("saved = %+v", got)
	}

	all, err := db.ListPreviewAppSettings(ctx)
	if err != nil || len(all) != 1 || all["web"].TTLHours != 12 {
		t.Errorf("ListPreviewAppSettings() = %+v, %v", all, err)
	}
}

func TestPreviewEnvironment_ForkFieldsAndCommentID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p := testPreviewEnvironment()
	p.IsFork, p.HeadRepo = true, "mallory/web"
	if err := db.SavePreviewEnvironment(ctx, p); err != nil {
		t.Fatalf("SavePreviewEnvironment() error = %v", err)
	}
	if err := db.SetPreviewEnvironmentCommentID(ctx, p.ID, 991); err != nil {
		t.Fatalf("SetPreviewEnvironmentCommentID() error = %v", err)
	}

	p.Status = PreviewStatusActive
	if err := db.UpdatePreviewEnvironment(ctx, p); err != nil {
		t.Fatalf("UpdatePreviewEnvironment() error = %v", err)
	}
	got, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if got.CommentID != 991 || !got.IsFork || got.HeadRepo != "mallory/web" || got.Status != PreviewStatusActive {
		t.Errorf("got %+v, want comment 991 kept across Update, fork fields intact", *got)
	}

	all, err := db.ListPreviewEnvironments(ctx)
	if err != nil || len(all) != 1 {
		t.Errorf("ListPreviewEnvironments() = %d rows, %v", len(all), err)
	}
}
