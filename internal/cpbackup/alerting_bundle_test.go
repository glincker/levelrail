package cpbackup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

func seedAlerting(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	adb, err := alerting.Open(ctx, filepath.Join(dir, AlertingFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = adb.Close() }()
	rule := alerting.Rule{
		ID: "r1", Name: "high cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: alerting.GreaterThan, Threshold: 80, ForDuration: time.Minute, NotifyKind: alerting.NotifySlack, Enabled: true,
	}
	if err := adb.SaveRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	ch := alerting.NotificationChannel{ID: "chn_1", Name: "ops", Kind: alerting.NotifyEmail, NotifyURL: "ops@example.com", Enabled: true}
	if err := adb.SaveNotificationChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
}

func TestBackupBundlesAlertingAndRestoreBringsItBack(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	seedAlerting(t, e.dir)

	m := e.backup()
	if m.Bundle != bundleTar || !m.IncludesAlerting {
		t.Fatalf("manifest bundle = %q includes_alerting = %v", m.Bundle, m.IncludesAlerting)
	}
	data, _ := e.srv.Data(m.Key)
	if len(data) == 0 {
		t.Fatal("no stored backup")
	}

	dest := t.TempDir()
	live := filepath.Join(dest, "levelrail.db")
	rep, err := Restore(context.Background(), NewBucketSource(newBucket(t, e.srv), m.Key), RestoreOptions{LivePath: live, Identities: []age.Identity{e.id}})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := readInstallID(context.Background(), live); got != m.InstallID {
		t.Errorf("restored install id = %q", got)
	}
	found := false
	for _, c := range rep.Checks {
		found = found || c.Name == "alerting"
	}
	if !found {
		t.Error("restore report has no alerting check")
	}
	ctx := context.Background()
	adb, err := alerting.Open(ctx, filepath.Join(dest, AlertingFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = adb.Close() }()
	if _, err := adb.GetRule(ctx, "r1"); err != nil {
		t.Errorf("rule missing after restore: %v", err)
	}
	if chans, err := adb.ListNotificationChannels(ctx); err != nil || len(chans) != 1 || chans[0].ID != "chn_1" {
		t.Errorf("channels after restore = %+v, err %v", chans, err)
	}
	if fi, err := os.Stat(filepath.Join(dest, AlertingFile)); err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("alerting.db mode = %v, err %v", fi, err)
	}
}

func TestBackupWithoutAlertingStillWorks(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	if m.Bundle != "" || m.IncludesAlerting {
		t.Fatalf("manifest bundle = %q includes_alerting = %v, want a bare database", m.Bundle, m.IncludesAlerting)
	}
}
