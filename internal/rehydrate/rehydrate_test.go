package rehydrate

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeInventory struct {
	dbs     []Database
	vols    []Volume
	backups []Backup
}

func (f fakeInventory) Databases(context.Context) ([]Database, error) { return f.dbs, nil }
func (f fakeInventory) Volumes(context.Context) ([]Volume, error)     { return f.vols, nil }
func (f fakeInventory) Backups(context.Context) ([]Backup, error)     { return f.backups, nil }

type fakeRestorer struct {
	calls   atomic.Int32
	failFor string
}

func (f *fakeRestorer) RestoreDatabase(_ context.Context, d Database, _ string) (string, error) {
	f.calls.Add(1)
	if d.Name == f.failFor {
		return "rsh_x", errors.New("boom")
	}
	return "rsh_" + d.Name, nil
}

func (f *fakeRestorer) RestoreVolume(_ context.Context, v Volume, _ string) (string, error) {
	f.calls.Add(1)
	return "rsh_" + v.Name, nil
}

var testNow = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func testInventory() fakeInventory {
	return fakeInventory{
		dbs:  []Database{{Name: "main", Engine: "postgres"}, {Name: "old", Engine: "postgres"}, {Name: "none", Engine: "postgres"}, {Name: "cache", Engine: "memcached", Stateless: true}},
		vols: []Volume{{App: "web", Name: "uploads"}},
		backups: []Backup{
			{ID: "b1", Kind: KindDatabase, Database: "main", Succeeded: true, FinishedAt: testNow.Add(-2 * time.Hour)},
			{ID: "b2", Kind: KindDatabase, Database: "main", Succeeded: true, FinishedAt: testNow.Add(-26 * time.Hour)},
			{ID: "b3", Kind: KindDatabase, Database: "main", Succeeded: false, FinishedAt: testNow.Add(-time.Hour)},
			{ID: "b4", Kind: KindDatabase, Database: "old", Succeeded: true, FinishedAt: testNow.Add(-30 * 24 * time.Hour)},
			{ID: "b5", Kind: KindVolume, App: "web", Volume: "uploads", Succeeded: true, FinishedAt: testNow.Add(-time.Hour)},
		},
	}
}

func TestBuildPlan_PicksLatestUsableAndBlocksStale(t *testing.T) {
	cfg := Config{MaxBackupAge: 7 * 24 * time.Hour, Concurrency: 2}
	rep, err := BuildPlan(context.Background(), testInventory(), cfg, PlanOptions{}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Item{}
	for _, it := range rep.Items {
		got[it.ID] = it
	}
	if it := got["database/main"]; it.Status != StatusPlanned || it.Backup.ID != "b1" {
		t.Fatalf("main = %+v, want planned from b1 (newest succeeded)", it)
	}
	if it := got["database/old"]; it.Status != StatusBlocked || it.Reason != ReasonBackupTooOld {
		t.Fatalf("old = %+v, want blocked too old", it)
	}
	if it := got["database/none"]; it.Status != StatusBlocked || it.Reason != ReasonNoBackup {
		t.Fatalf("none = %+v, want blocked no backup", it)
	}
	if it := got["database/cache"]; it.Status != StatusSkipped {
		t.Fatalf("cache = %+v, want skipped", it)
	}
	if it := got["volume/web/uploads"]; it.Status != StatusPlanned || it.Backup.ID != "b5" {
		t.Fatalf("volume = %+v, want planned from b5", it)
	}
	if rep.Summary.Restorable != 2 || rep.Summary.Blocked != 2 || rep.Summary.Skipped != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if rep.Summary.WorstBackupAgeSeconds != int64(2*time.Hour/time.Second) {
		t.Fatalf("worst age = %d", rep.Summary.WorstBackupAgeSeconds)
	}
}

func TestBuildPlan_AllowStaleKeepsOldBackup(t *testing.T) {
	cfg := Config{MaxBackupAge: 7 * 24 * time.Hour, Concurrency: 2}
	rep, err := BuildPlan(context.Background(), testInventory(), cfg, PlanOptions{AllowStale: true}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range rep.Items {
		if it.ID == "database/old" && it.Status != StatusPlanned {
			t.Fatalf("old = %+v, want planned with allow stale", it)
		}
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{EnvMaxBackupAge: "48h", EnvConcurrency: "5"}
	cfg, err := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil || cfg.MaxBackupAge != 48*time.Hour || cfg.Concurrency != 5 {
		t.Fatalf("cfg = %+v err = %v", cfg, err)
	}
	env[EnvConcurrency] = "0"
	if _, err := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil {
		t.Fatal("zero concurrency must be rejected")
	}
}

func TestServiceRun_PartialReportWithPerResourceStatus(t *testing.T) {
	inv := testInventory()
	inv.dbs = []Database{{Name: "main", Engine: "postgres"}, {Name: "bad", Engine: "postgres"}}
	inv.backups = append(inv.backups, Backup{ID: "b9", Kind: KindDatabase, Database: "bad", Succeeded: true, FinishedAt: testNow.Add(-time.Hour)})
	fr := &fakeRestorer{failFor: "bad"}
	svc := &Service{Inventory: inv, Restorer: fr, Config: Config{MaxBackupAge: 7 * 24 * time.Hour, Concurrency: 2}, Now: func() time.Time { return testNow }}

	if _, err := svc.Start(context.Background(), PlanOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(context.Background(), PlanOptions{}); err != nil && !errors.Is(err, ErrBusy) {
		t.Fatalf("second start err = %v", err)
	}
	var rep Report
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rep, _, _ = svc.Report(context.Background())
		if rep.State != StateRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rep.State != StatePartial {
		t.Fatalf("state = %q, want partial", rep.State)
	}
	status := map[string]string{}
	for _, it := range rep.Items {
		status[it.ID] = it.Status
	}
	if status["database/main"] != StatusSucceeded || status["database/bad"] != StatusFailed || status["volume/web/uploads"] != StatusSucceeded {
		t.Fatalf("statuses = %v", status)
	}
	if fr.calls.Load() != 3 {
		t.Fatalf("restorer calls = %d, want 3", fr.calls.Load())
	}
}

func TestServiceStart_NotConfigured(t *testing.T) {
	svc := &Service{Inventory: fakeInventory{}, Config: Config{Concurrency: 1}}
	if _, err := svc.Start(context.Background(), PlanOptions{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}
