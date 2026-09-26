package cpbackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

func TestRunBackup_LayoutManifestAndRoundTrip(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()

	wantKey := DataKey(m.InstallID, time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC))
	if m.Key != wantKey || !strings.HasPrefix(m.Key, "cp-backups/inst-") || !strings.HasSuffix(m.Key, "/2026/09/25/20260925T020000Z.db.age") {
		t.Fatalf("key = %q", m.Key)
	}
	keys := e.srv.Keys()
	if len(keys) != 2 || keys[0] != m.Key || keys[1] != ManifestKey(m.Key) {
		t.Fatalf("stored keys = %v", keys)
	}
	if m.IncludesMasterKey || !m.ContainsWrappedSecrets || m.SchemaVersion == 0 || m.MigrationsApplied == 0 || m.BinaryVersion != "v-test" || m.RecipientCount != 1 {
		t.Fatalf("manifest = %+v", m)
	}
	data, _ := e.srv.Data(m.Key)
	if strings.Contains(string(data), "SQLite format") {
		t.Fatal("stored object is not encrypted")
	}
	if s := e.settings(); s.LastBackupKey != m.Key || s.LastBackupError != "" || s.InstallID != m.InstallID {
		t.Fatalf("settings = %+v", s)
	}

	live := filepath.Join(t.TempDir(), "levelrail.db")
	rep, err := Restore(context.Background(), NewBucketSource(newBucket(t, e.srv), m.Key), RestoreOptions{LivePath: live, Identities: []age.Identity{e.id}})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if rep.SchemaVersion != m.SchemaVersion {
		t.Errorf("schema = %d, want %d", rep.SchemaVersion, m.SchemaVersion)
	}
	if got := readInstallID(context.Background(), live); got != m.InstallID {
		t.Errorf("restored install id = %q, want %q", got, m.InstallID)
	}
}

func TestRunBackup_NotConfigured(t *testing.T) {
	e := newDREnv(t)
	if _, err := e.svc.RunBackup(context.Background(), time.Time{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestRunBackup_SlotIsIdempotent(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	slot := time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC)
	first, err := e.svc.RunBackup(context.Background(), slot)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Hour)
	second, err := e.svc.RunBackup(context.Background(), slot)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key || first.SHA256 != second.SHA256 || len(e.srv.Keys()) != 2 {
		t.Fatalf("second run re-uploaded: keys = %v", e.srv.Keys())
	}
}

func TestRunBackup_ResumesAfterFailedUpload(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	slot := time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC)

	e.srv.FailPuts = true
	if _, err := e.svc.RunBackup(context.Background(), slot); err == nil {
		t.Fatal("expected upload failure")
	}
	if s := e.settings(); s.LastBackupError == "" || !s.LastBackupAt.IsZero() {
		t.Fatalf("failure not recorded: %+v", s)
	}

	e.srv.FailPuts = false
	e.srv.Put(DataKey(e.settings().InstallID, slot), []byte("half an upload"), e.clock.Now())
	m, err := e.svc.RunBackup(context.Background(), slot)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if s := e.settings(); s.LastBackupError != "" || s.LastBackupKey != m.Key {
		t.Fatalf("success not recorded: %+v", s)
	}
	live := filepath.Join(t.TempDir(), "levelrail.db")
	if _, err := Restore(context.Background(), NewBucketSource(newBucket(t, e.srv), m.Key), RestoreOptions{LivePath: live, Identities: []age.Identity{e.id}}); err != nil {
		t.Fatalf("resumed backup does not restore: %v", err)
	}
}

func TestRunBackup_RejectsBusy(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	e.svc.backupMu.Lock()
	defer e.svc.backupMu.Unlock()
	if _, err := e.svc.RunBackup(context.Background(), time.Time{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
}

func TestRunBackup_PrunesBoundedPerRun(t *testing.T) {
	e := newDREnv(t)
	e.svc.Opts.Retention = Retention{Daily: 1, Weekly: 1, Monthly: 1}
	e.svc.Opts.MaxPrune = 2
	e.configure(nil)
	id := "inst-old"
	if err := e.db.SetCPDRInstallID(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	for d := 1; d <= 6; d++ {
		ts := time.Date(2026, 8, d, 2, 0, 0, 0, time.UTC)
		e.srv.Put(DataKey(id, ts), []byte("x"), ts)
		e.srv.Put(ManifestKey(DataKey(id, ts)), []byte("{}"), ts)
	}
	e.backup()
	remotes, err := listRemotes(context.Background(), newBucket(t, e.srv), installPrefix(id))
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 6+1-2 {
		t.Fatalf("after bounded prune %d backups remain, want 5", len(remotes))
	}
	e.clock.Advance(24 * time.Hour)
	e.backup()
	e.clock.Advance(24 * time.Hour)
	e.backup()
	remotes, _ = listRemotes(context.Background(), newBucket(t, e.srv), installPrefix(id))
	for _, r := range remotes {
		if !r.Complete {
			t.Errorf("prune left an orphan: %+v", r)
		}
	}
}

func TestOptionsFromEnv(t *testing.T) {
	env := map[string]string{
		EnvOffboxSchedule: "0 3 * * *", EnvRetainDaily: "9", EnvRetainWeekly: "bogus", EnvGrace: "2h", EnvDrillSchedule: "not cron",
	}
	o := OptionsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if o.Schedule != "0 3 * * *" || o.Retention.Daily != 9 || o.Retention.Weekly != 4 || o.Grace != 2*time.Hour || o.DrillSchedule != "0 5 * * 0" {
		t.Fatalf("options = %+v", o)
	}
}

func TestTick_SchedulesAndRetries(t *testing.T) {
	e := newDREnv(t)
	e.configure(func(u *ConfigUpdate) { u.Schedule = "0 2 * * *" })
	ctx := context.Background()

	e.svc.Tick(ctx)
	first := e.settings()
	if first.LastBackupKey == "" || first.LastDrillAt.IsZero() {
		t.Fatalf("first tick should back up and drill: %+v", first)
	}
	e.svc.Tick(ctx)
	if got := len(e.srv.Keys()); got != 2 {
		t.Fatalf("second tick uploaded again: %d keys", got)
	}

	e.clock.Advance(24 * time.Hour)
	e.svc.Tick(ctx)
	if got := len(e.srv.Keys()); got != 4 {
		t.Fatalf("tick at next slot: %d keys, want 4", got)
	}
	if !strings.Contains(e.settings().LastBackupKey, "20260926T020000Z") {
		t.Errorf("slot key = %q", e.settings().LastBackupKey)
	}

	e.clock.Advance(24 * time.Hour)
	e.srv.FailPuts = true
	e.svc.Tick(ctx)
	if e.settings().LastBackupError == "" {
		t.Fatal("failure not recorded")
	}
	attempt := e.settings().LastAttemptAt
	e.clock.Advance(time.Minute)
	e.svc.Tick(ctx)
	if !e.settings().LastAttemptAt.Equal(attempt) {
		t.Error("retried before RetryAfter")
	}
	e.srv.FailPuts = false
	e.clock.Advance(e.svc.Opts.RetryAfter)
	e.svc.Tick(ctx)
	if e.settings().LastBackupError != "" {
		t.Errorf("retry did not recover: %s", e.settings().LastBackupError)
	}
}

func TestUpdateConfig_Validation(t *testing.T) {
	e := newDREnv(t)
	good := e.id.Recipient().String()
	cases := []struct {
		name string
		u    ConfigUpdate
	}{
		{"bad cron", ConfigUpdate{Schedule: "nope"}},
		{"bad recipient", ConfigUpdate{Recipients: []string{"age1notakey"}}},
		{"private key as recipient", ConfigUpdate{Recipients: []string{e.id.String()}}},
		{"unknown target", ConfigUpdate{TargetID: "ghost"}},
		{"negative retention", ConfigUpdate{Retention: Retention{Daily: -1}}},
		{"enabled without recipient", ConfigUpdate{Enabled: true, TargetID: "main"}},
		{"enabled without target", ConfigUpdate{Enabled: true, Recipients: []string{good}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := e.svc.UpdateConfig(context.Background(), tc.u); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	if _, err := os.Stat(e.dir); err != nil {
		t.Fatal(err)
	}
}

func TestCleanTemp_RemovesLeftoverWorkDirs(t *testing.T) {
	e := newDREnv(t)
	stale := filepath.Join(e.dir, DirName, ".offbox-crashed")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stale, "snapshot.db"), "plaintext snapshot")
	keep := filepath.Join(e.dir, DirName, "levelrail-20260101T000000Z.db")
	writeFile(t, keep, "a real local snapshot")

	e.svc.CleanTemp()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale work directory survived")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("cleanup removed a real snapshot")
	}
	e.configure(nil)
	e.backup()
	assertNoLeftovers(t, filepath.Join(e.dir, DirName), "levelrail-20260101T000000Z.db")
}
