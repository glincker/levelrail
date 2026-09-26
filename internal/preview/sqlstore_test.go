package preview

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func openSQLStore(t *testing.T) (*SQLStore, *store.DB) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLStore(db), db
}

func TestSQLStore_SettingsRoundTrip(t *testing.T) {
	s, _ := openSQLStore(t)
	ctx := context.Background()
	got, err := s.GetPreviewSettings(ctx, "web")
	if err != nil || got.Enabled || got.Path != "/" {
		t.Fatalf("defaults = %+v err %v", got, err)
	}
	want := AppSettings{App: "web", Enabled: true, Path: "/pricing", WaitMS: 250}
	if err := s.SavePreviewSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetPreviewSettings(ctx, "web"); got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestSQLStore_RecordsUpsertListDelete(t *testing.T) {
	s, _ := openSQLStore(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	r1 := Record{DeploymentID: "dep_1", App: "web", Path: "/", Bytes: 100, Width: 640, Height: 400, Status: StatusOK, HTTPStatus: 200, CapturedAt: base}
	r2 := Record{DeploymentID: "dep_2", App: "web", Path: "/", Status: StatusSkipped, Reason: ReasonAuthWall, Detail: "login", CapturedAt: base.Add(time.Hour)}
	r3 := Record{DeploymentID: "dep_3", App: "api", Path: "/", Status: StatusOK, CapturedAt: base}
	for _, r := range []Record{r1, r2, r3} {
		if err := s.UpsertPreviewRecord(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListPreviewRecords(ctx, "web")
	if err != nil || len(list) != 2 || list[0].DeploymentID != "dep_2" {
		t.Fatalf("list = %+v err %v, want dep_2 then dep_1", list, err)
	}
	if list[0].Reason != ReasonAuthWall || list[1].Bytes != 100 || !list[1].CapturedAt.Equal(base) {
		t.Errorf("fields did not round trip: %+v", list)
	}
	if err := s.TouchPreviewViewed(ctx, "dep_1", base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	r1.Bytes = 200
	if err := s.UpsertPreviewRecord(ctx, r1); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPreviewRecord(ctx, "dep_1")
	if err != nil || got == nil || got.Bytes != 200 || got.LastViewedAt.IsZero() {
		t.Errorf("upsert must keep the view time: %+v err %v", got, err)
	}
	if missing, err := s.GetPreviewRecord(ctx, "nope"); err != nil || missing != nil {
		t.Errorf("missing record = %+v err %v, want nil", missing, err)
	}
	all, _ := s.ListAllPreviewRecords(ctx)
	if len(all) != 3 {
		t.Errorf("all = %d, want 3", len(all))
	}
	if err := s.DeletePreviewRecords(ctx, []string{"dep_1", "dep_2"}); err != nil {
		t.Fatal(err)
	}
	if all, _ = s.ListAllPreviewRecords(ctx); len(all) != 1 || all[0].DeploymentID != "dep_3" {
		t.Errorf("after delete = %+v", all)
	}
	if err := s.DeletePreviewRecords(ctx, nil); err != nil {
		t.Errorf("deleting nothing failed: %v", err)
	}
}

func TestSQLStore_StateAndLatestDeployment(t *testing.T) {
	s, db := openSQLStore(t)
	ctx := context.Background()
	if v, err := s.GetPreviewState(ctx, "k"); err != nil || v != "" {
		t.Fatalf("unset state = %q err %v", v, err)
	}
	if err := s.SetPreviewState(ctx, "k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPreviewState(ctx, "k", "v2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetPreviewState(ctx, "k"); v != "v2" {
		t.Errorf("state = %q, want v2", v)
	}

	now := time.Now()
	for i, a := range []store.DeployAttempt{
		{ID: "dep_old", ServiceName: "web", Image: "web:1", Status: store.DeployAttemptStatusSucceeded, StartedAt: now.Add(-2 * time.Hour)},
		{ID: "dep_new", ServiceName: "web", Image: "web:2", Status: store.DeployAttemptStatusSucceeded, StartedAt: now.Add(-time.Hour)},
		{ID: "dep_fail", ServiceName: "web", Image: "web:3", Status: store.DeployAttemptStatusFailed, StartedAt: now},
	} {
		a.Source = store.DeployAttemptSourceImage
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed attempt %d: %v", i, err)
		}
	}
	for _, tc := range []struct{ image, want string }{{"", "dep_new"}, {"web:1", "dep_old"}, {"web:3", ""}, {"web:9", ""}} {
		got, err := s.LatestSucceededDeployment(ctx, "web", tc.image)
		if err != nil || got != tc.want {
			t.Errorf("LatestSucceededDeployment(%q) = %q err %v, want %q", tc.image, got, err, tc.want)
		}
	}
}
