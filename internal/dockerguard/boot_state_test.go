package dockerguard

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func bootOnce(t *testing.T, d *fakeDaemon, dir string, env map[string]string) Booted {
	t.Helper()
	all := map[string]string{"DOCKER_HOST": "unix://" + d.socket}
	for k, v := range env {
		all[k] = v
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b, err := Boot(ctx, BootConfig{DataDir: dir, Lookup: envMap(all)})
	if err != nil {
		t.Fatal(err)
	}
	if b.Server != nil {
		t.Cleanup(func() { _ = b.Server.Close() })
	}
	return b
}

func TestBootRecordsWhatRanAndRestartsTheWindow(t *testing.T) {
	d := newFakeDaemon(t)
	dir := shortTempDir(t)
	path := filepath.Join(dir, SettingsFile)
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := SaveSettings(path, Settings{AuditSince: march, LastRunMode: ModeOff}); err != nil {
		t.Fatal(err)
	}
	bootOnce(t, d, dir, map[string]string{EnvMode: "audit"})
	s, err := LoadSettings(path)
	if err != nil || s.LastRunMode != ModeAudit || !s.AuditSince.After(march) {
		t.Fatalf("after an audit boot: %+v %v", s, err)
	}
	since := s.AuditSince
	bootOnce(t, d, dir, map[string]string{EnvMode: "audit"})
	if s, _ = LoadSettings(path); !s.AuditSince.Equal(since) {
		t.Fatalf("audit to audit boot moved the window: %v to %v", since, s.AuditSince)
	}
	bootOnce(t, d, dir, map[string]string{EnvMode: "off"})
	if s, _ = LoadSettings(path); s.LastRunMode != ModeOff || !s.AuditSince.IsZero() {
		t.Fatalf("after an off boot: %+v", s)
	}
}

func TestCorruptSettingsFileIsNeverOverwritten(t *testing.T) {
	d := newFakeDaemon(t)
	dir := shortTempDir(t)
	path := filepath.Join(dir, SettingsFile)
	corrupt := []byte(`{"mode": "enforce", "audit_si`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	for boot := 1; boot <= 2; boot++ {
		b := bootOnce(t, d, dir, nil)
		st := b.Controller.Status()
		if st.Effective != ModeEnforce || st.ConfigError == "" {
			t.Fatalf("boot %d: effective %q config error %q, want enforce with the warning", boot, st.Effective, st.ConfigError)
		}
		if _, err := b.Controller.SetMode(ModeAudit, "admin", time.Now()); err == nil {
			t.Fatalf("boot %d: SetMode must refuse while the settings file is unreadable", boot)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, corrupt) { //nolint:gosec // test path
			t.Fatalf("boot %d: the corrupt file was rewritten: %q", boot, got)
		}
		_ = b.Server.Close()
	}
}

func TestLiveSwitchesKeepWhatTheGuardActuallyRuns(t *testing.T) {
	boot := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	t.Run("enforce then off records enforce as the running mode", func(t *testing.T) {
		h := startGuard(t, ModeEnforce)
		path := filepath.Join(t.TempDir(), SettingsFile)
		c := NewController(path, envMap(nil), h.server, "", nil)
		if _, err := c.SetMode(ModeOff, "admin", boot); err != nil {
			t.Fatal(err)
		}
		if s, _ := LoadSettings(path); s.LastRunMode != ModeEnforce {
			t.Fatalf("LastRunMode = %q, want enforce", s.LastRunMode)
		}
	})
	t.Run("audit off audit keeps the window while the guard never stopped", func(t *testing.T) {
		h := startGuard(t, ModeAudit)
		c := NewController(filepath.Join(t.TempDir(), SettingsFile), envMap(nil), h.server, "", nil)
		if err := c.NoteBoot(boot); err != nil {
			t.Fatal(err)
		}
		if _, err := c.SetMode(ModeOff, "admin", boot.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if st, _ := c.SetMode(ModeAudit, "admin", boot.Add(2*time.Hour)); !st.AuditSince.Equal(boot) {
			t.Fatalf("AuditSince = %v, want %v", st.AuditSince, boot)
		}
	})
}
