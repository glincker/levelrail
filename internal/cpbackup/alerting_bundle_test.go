package cpbackup

import (
	"archive/tar"
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

func TestRestoreRollsAlertingBackWhenMainCommitFails(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	seedAlerting(t, e.dir)
	m := e.backup()

	dest := t.TempDir()
	old := []byte("previous alerting contents")
	if err := os.WriteFile(filepath.Join(dest, AlertingFile), old, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := RestoreOptions{LivePath: filepath.Join(dest, "levelrail.db"), Identities: []age.Identity{e.id}}
	opts.hook = func(stage string) error {
		if stage == "before-main-commit" {
			return os.ErrPermission
		}
		return nil
	}
	if _, err := Restore(context.Background(), NewBucketSource(newBucket(t, e.srv), m.Key), opts); err == nil {
		t.Fatal("restore succeeded, want failure")
	}
	got, err := os.ReadFile(filepath.Join(dest, AlertingFile)) //nolint:gosec // test temp dir
	if err != nil || string(got) != string(old) {
		t.Fatalf("alerting.db after failed restore = %q, err %v; want the previous file", got, err)
	}
}

func TestRestoreBareBackupKeepsLiveAlerting(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, AlertingFile), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := Restore(context.Background(), NewBucketSource(newBucket(t, e.srv), m.Key), RestoreOptions{LivePath: filepath.Join(dest, "levelrail.db"), Identities: []age.Identity{e.id}})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, AlertingFile)); //nolint:gosec // test temp dir
	string(b) != "keep" {
		t.Error("live alerting.db was replaced")
	}
	found := false
	for _, c := range rep.Checks {
		found = found || (c.Name == "alerting" && c.Detail == "alerting.db not in backup, current file kept")
	}
	if !found {
		t.Error("no note that alerting.db was kept")
	}
}

func TestUnpackBundleRejectsBadEntries(t *testing.T) {
	build := func(entries ...tar.Header) string {
		path := filepath.Join(t.TempDir(), "b.tar")
		f, _ := os.Create(path) //nolint:gosec // test temp dir
		tw := tar.NewWriter(f)
		for _, h := range entries {
			if err := tw.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
			if h.Typeflag == tar.TypeReg {
				_, _ = tw.Write(make([]byte, h.Size))
			}
		}
		_ = tw.Close()
		_ = f.Close()
		return path
	}
	reg := func(name string, size int64) tar.Header {
		return tar.Header{Name: name, Mode: 0o600, Size: size, Typeflag: tar.TypeReg}
	}
	tests := map[string]struct {
		path string
		max  int64
	}{
		"symlink":   {build(tar.Header{Name: mainEntry, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}), 1 << 20},
		"duplicate": {build(reg(mainEntry, 4), reg(mainEntry, 4)), 1 << 20},
		"oversize":  {build(reg(mainEntry, 100), reg(AlertingFile, 100)), 150},
	}
	for name, tc := range tests {
		if _, _, err := unpackBundle(tc.path, t.TempDir(), tc.max); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCorruptAlertingStillShipsMainBackup(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	if err := os.WriteFile(filepath.Join(e.dir, AlertingFile), []byte("not a sqlite file at all, just junk bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := e.backup()
	if m.IncludesAlerting || m.Bundle != "" || m.AlertingWarning == "" || m.ManifestVersion != manifestFormat {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestBundleManifestUsesNewerFormat(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	seedAlerting(t, e.dir)
	if m := e.backup(); m.ManifestVersion != manifestBundleFormat {
		t.Fatalf("manifest version = %d", m.ManifestVersion)
	}
}
