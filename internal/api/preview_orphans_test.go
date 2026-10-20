package api

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedPreviewRow(t *testing.T, db *store.DB, id, app string, pr int, status string, updated time.Time) {
	t.Helper()
	ts := updated.UTC().Format(time.RFC3339Nano)
	err := db.SavePreviewEnvironment(context.Background(), store.PreviewEnvironment{
		ID: id, AppName: app, PRNumber: pr, PreviewAppID: previewAppName(app, pr), Branch: "b", HeadSHA: "s",
		Status: status, CreatedAt: ts, UpdatedAt: ts,
	})
	if err != nil {
		t.Fatalf("seed preview %q: %v", id, err)
	}
}

func seedPreviewTaggedService(t *testing.T, db *store.DB, name, envID string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:v1", Port: 3000}); err != nil {
		t.Fatalf("seed service %q: %v", name, err)
	}
	if envID != "" {
		if err := db.SetServiceEnvironment(ctx, name, envID); err != nil {
			t.Fatalf("tag service %q: %v", name, err)
		}
	}
}

func TestSweepOrphanPreviews_MarksStuckDeploysFailedOnly(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedPreviewRow(t, db, "stuck", "web", 1, store.PreviewStatusDeploying, time.Now().Add(-2*time.Hour))
	seedPreviewRow(t, db, "fresh", "web", 2, store.PreviewStatusDeploying, time.Now())
	seedPreviewRow(t, db, "live", "web", 3, store.PreviewStatusActive, time.Now().Add(-2*time.Hour))

	res, err := rt.SweepOrphanPreviews(ctx)
	if err != nil || res.StuckDeploys != 1 {
		t.Fatalf("SweepOrphanPreviews() = %+v, %v, want exactly the stuck deploy repaired", res, err)
	}

	want := map[int]string{1: store.PreviewStatusFailed, 2: store.PreviewStatusDeploying, 3: store.PreviewStatusActive}
	for pr, status := range want {
		p, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", pr)
		if err != nil || p.Status != status {
			t.Errorf("pr %d: status = %v (err %v), want %s", pr, p, err, status)
		}
	}

	res, err = rt.SweepOrphanPreviews(ctx)
	if err != nil || res.StuckDeploys != 0 {
		t.Errorf("second sweep = %+v, %v, want nothing left to repair", res, err)
	}
}

func TestSweepOrphanPreviews_RemovesOnlyUnownedPreviewServices(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := db.SaveProject(ctx, store.Project{ID: "preview-web", Name: "web previews", CreatedAt: now}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "preview-env-web", ProjectID: "preview-web", Name: "Preview", CreatedAt: now}); err != nil {
		t.Fatalf("save environment: %v", err)
	}

	seedPreviewRow(t, db, "owned", "web", 3, store.PreviewStatusActive, time.Now())
	seedPreviewTaggedService(t, db, "web-pr-3", "preview-env-web")
	seedPreviewTaggedService(t, db, "web-pr-3-worker", "preview-env-web")
	seedPreviewTaggedService(t, db, "web-pr-9", "preview-env-web")
	seedPreviewTaggedService(t, db, "web-pr-x", "preview-env-web")
	seedPreviewTaggedService(t, db, "web-pr-9-untagged", "")
	seedPreviewTaggedService(t, db, "billing", "preview-env-web")

	res, err := rt.SweepOrphanPreviews(ctx)
	if err != nil || res.OrphanedServices != 1 {
		t.Fatalf("SweepOrphanPreviews() = %+v, %v, want exactly web-pr-9 removed", res, err)
	}

	for name, wantGone := range map[string]bool{
		"web-pr-9": true, "web-pr-3": false, "web-pr-3-worker": false,
		"web-pr-x": false, "web-pr-9-untagged": false, "billing": false,
	} {
		_, err := db.GetDesiredService(ctx, name)
		if gone := err != nil; gone != wantGone {
			t.Errorf("service %q gone = %v, want %v", name, gone, wantGone)
		}
	}
}

func TestSweepStalePreviewEnvironments_HonorsPerAppTTL(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedPreviewRow(t, db, "short", "web", 1, store.PreviewStatusActive, time.Now().Add(-2*time.Hour))
	seedPreviewRow(t, db, "default", "other", 1, store.PreviewStatusActive, time.Now().Add(-2*time.Hour))
	seedPreviewRow(t, db, "held", "web", 2, store.PreviewStatusAwaitingApproval, time.Now().Add(-3*time.Hour))
	if err := db.SavePreviewAppSettings(ctx, store.PreviewAppSettings{AppName: "web", OnLimit: store.PreviewOnLimitEvictOldest, TTLHours: 1}); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	swept, err := rt.SweepStalePreviewEnvironments(ctx)
	if err != nil || swept != 2 {
		t.Fatalf("SweepStalePreviewEnvironments() = %d, %v, want the two web rows past their 1h TTL", swept, err)
	}
	if _, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "other", 1); err != nil {
		t.Errorf("other app's preview (default 7d TTL) was swept: %v", err)
	}
}
