package store

import (
	"context"
	"errors"
	"testing"
)

func testPreviewEnvironment() PreviewEnvironment {
	return PreviewEnvironment{
		ID: "prev_test1", AppName: "web", PRNumber: 42, PreviewAppID: "web-pr-42",
		Branch: "feature-x", HeadSHA: "abc123", Domain: "pr-42.web.example.com",
		Status: PreviewStatusDeploying, CreatedAt: "2026-08-20T00:00:00Z", UpdatedAt: "2026-08-20T00:00:00Z",
	}
}

func TestSaveAndGetPreviewEnvironmentByAppAndPR(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	want := testPreviewEnvironment()

	if err := db.SavePreviewEnvironment(ctx, want); err != nil {
		t.Fatalf("SavePreviewEnvironment() error = %v", err)
	}

	got, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if *got != want {
		t.Errorf("GetPreviewEnvironmentByAppAndPR() = %+v, want %+v", *got, want)
	}
}

func TestGetPreviewEnvironmentByAppAndPR_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 999)
	if !errors.Is(err, ErrPreviewEnvironmentNotFound) {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v, want ErrPreviewEnvironmentNotFound", err)
	}
}

func TestUpdatePreviewEnvironment(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seeded := testPreviewEnvironment()
	if err := db.SavePreviewEnvironment(ctx, seeded); err != nil {
		t.Fatalf("SavePreviewEnvironment() error = %v", err)
	}

	updated := seeded
	updated.HeadSHA = "def456"
	updated.Status = PreviewStatusActive
	updated.StatusReason = "domain conflict, deployed without a domain"
	updated.EnvironmentID = "preview-env-web"
	updated.UpdatedAt = "2026-08-21T00:00:00Z"

	if err := db.UpdatePreviewEnvironment(ctx, updated); err != nil {
		t.Fatalf("UpdatePreviewEnvironment() error = %v", err)
	}

	got, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if *got != updated {
		t.Errorf("GetPreviewEnvironmentByAppAndPR() = %+v, want %+v", *got, updated)
	}
}

func TestUpdatePreviewEnvironment_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdatePreviewEnvironment(context.Background(), testPreviewEnvironment())
	if !errors.Is(err, ErrPreviewEnvironmentNotFound) {
		t.Fatalf("UpdatePreviewEnvironment() error = %v, want ErrPreviewEnvironmentNotFound", err)
	}
}

func TestListPreviewEnvironmentsByApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	pr1 := testPreviewEnvironment()
	pr2 := testPreviewEnvironment()
	pr2.ID, pr2.PRNumber, pr2.PreviewAppID = "prev_test2", 7, "web-pr-7"
	other := testPreviewEnvironment()
	other.ID, other.AppName, other.PreviewAppID = "prev_test3", "api", "api-pr-42"

	for _, p := range []PreviewEnvironment{pr1, pr2, other} {
		if err := db.SavePreviewEnvironment(ctx, p); err != nil {
			t.Fatalf("SavePreviewEnvironment(%q) error = %v", p.ID, err)
		}
	}

	got, err := db.ListPreviewEnvironmentsByApp(ctx, "web")
	if err != nil {
		t.Fatalf("ListPreviewEnvironmentsByApp() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListPreviewEnvironmentsByApp() returned %d rows, want 2", len(got))
	}
	// Newest (highest PR number) first.
	if got[0].PRNumber != 42 || got[1].PRNumber != 7 {
		t.Errorf("ListPreviewEnvironmentsByApp() order = [%d, %d], want [42, 7]", got[0].PRNumber, got[1].PRNumber)
	}
}

func TestDeletePreviewEnvironment(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seeded := testPreviewEnvironment()
	if err := db.SavePreviewEnvironment(ctx, seeded); err != nil {
		t.Fatalf("SavePreviewEnvironment() error = %v", err)
	}

	if err := db.DeletePreviewEnvironment(ctx, seeded.ID); err != nil {
		t.Fatalf("DeletePreviewEnvironment() error = %v", err)
	}

	_, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if !errors.Is(err, ErrPreviewEnvironmentNotFound) {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() after delete error = %v, want ErrPreviewEnvironmentNotFound", err)
	}
}

func TestDeletePreviewEnvironment_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.DeletePreviewEnvironment(context.Background(), "prev_missing")
	if !errors.Is(err, ErrPreviewEnvironmentNotFound) {
		t.Fatalf("DeletePreviewEnvironment() error = %v, want ErrPreviewEnvironmentNotFound", err)
	}
}

func TestNewPreviewEnvironmentID_Unique(t *testing.T) {
	a, err := NewPreviewEnvironmentID()
	if err != nil {
		t.Fatalf("NewPreviewEnvironmentID() error = %v", err)
	}
	b, err := NewPreviewEnvironmentID()
	if err != nil {
		t.Fatalf("NewPreviewEnvironmentID() error = %v", err)
	}
	if a == b {
		t.Errorf("NewPreviewEnvironmentID() returned the same ID twice: %q", a)
	}
}

func TestSetGitSourcePreviewEnabled(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://example.com/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatalf("SaveGitSource() error = %v", err)
	}

	got, err := db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if got.PreviewEnabled {
		t.Fatalf("GetGitSource().PreviewEnabled = true, want false by default")
	}

	if err := db.SetGitSourcePreviewEnabled(ctx, "web", true); err != nil {
		t.Fatalf("SetGitSourcePreviewEnabled() error = %v", err)
	}

	got, err = db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if !got.PreviewEnabled {
		t.Errorf("GetGitSource().PreviewEnabled = false, want true after SetGitSourcePreviewEnabled")
	}

	// An unrelated SaveGitSource edit (a normal connect-form re-save)
	// must never silently flip this back off, the same "ordinary edit
	// can't move it" invariant every other dedicated-setter field in
	// this package already guarantees.
	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://example.com/web2.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatalf("SaveGitSource() re-save error = %v", err)
	}
	got, err = db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if !got.PreviewEnabled {
		t.Errorf("GetGitSource().PreviewEnabled = false after an unrelated SaveGitSource, want it to survive unchanged")
	}
}

func TestSetGitSourcePreviewEnabled_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.SetGitSourcePreviewEnabled(context.Background(), "missing", true)
	if !errors.Is(err, ErrGitSourceNotFound) {
		t.Fatalf("SetGitSourcePreviewEnabled() error = %v, want ErrGitSourceNotFound", err)
	}
}
