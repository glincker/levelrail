package preview

import (
	"context"
	"testing"
	"time"
)

func TestSQLStore_ModeStorageAndLegacyRows(t *testing.T) {
	s, db := openSQLStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		app     string
		enabled int
		want    Mode
	}{{"on", 1, ModeScreenshot}, {"off", 0, ModeOff}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO deploy_preview_settings (app_name, enabled, path, wait_ms, updated_at) VALUES (?, ?, '/', 0, '')`, tc.app, tc.enabled); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetPreviewSettings(ctx, tc.app)
		if err != nil || got.Mode != tc.want {
			t.Errorf("legacy row enabled=%d read as %+v err %v, want mode %s", tc.enabled, got, err, tc.want)
		}
	}
	for _, mode := range []Mode{ModeMetadata, ModeOff, ModeScreenshot} {
		if err := s.SavePreviewSettings(ctx, AppSettings{App: "m", Mode: mode, Path: "/"}); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.GetPreviewSettings(ctx, "m"); got.Mode != mode || got.Enabled != (mode != ModeOff) {
			t.Errorf("round trip of %s = %+v", mode, got)
		}
	}
	var legacy bool
	if err := db.QueryRowContext(ctx, `SELECT enabled FROM deploy_preview_settings WHERE app_name = 'm'`).Scan(&legacy); err != nil || !legacy {
		t.Errorf("enabled column = %v err %v, want true only for screenshot", legacy, err)
	}
}

func TestSQLStore_RecordSourceAndMeta(t *testing.T) {
	s, _ := openSQLStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.UpsertPreviewRecord(ctx, Record{DeploymentID: "a", App: "web", Status: StatusOK, Source: SourceCard, Meta: `{"title":"x"}`, CapturedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPreviewRecord(ctx, Record{DeploymentID: "b", App: "web", Status: StatusSkipped, CapturedAt: now}); err != nil {
		t.Fatal(err)
	}
	a, _ := s.GetPreviewRecord(ctx, "a")
	b, _ := s.GetPreviewRecord(ctx, "b")
	if a.Source != SourceCard || a.Meta != `{"title":"x"}` || b.Source != SourceScreenshot {
		t.Errorf("records = %+v / %+v", a, b)
	}
}
