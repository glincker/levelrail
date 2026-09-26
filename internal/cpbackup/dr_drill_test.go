package cpbackup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

func TestRunDrill_FullPass(t *testing.T) {
	e := newDREnv(t)
	e.svc.DrillIdentities = []age.Identity{e.id}
	e.configure(func(u *ConfigUpdate) { u.Recipients = []string{mustIdentity(t).Recipient().String()} })
	m := e.backup()

	res, err := e.svc.RunDrill(context.Background())
	if err != nil || !res.OK || res.Partial || res.BackupKey != m.Key {
		t.Fatalf("drill = %+v, err = %v", res, err)
	}
	names := map[string]bool{}
	for _, c := range res.Checks {
		names[c.Name] = c.OK
	}
	for _, want := range []string{"manifest", "checksum", "decrypt", "integrity", "schema_version", "row_counts"} {
		if !names[want] {
			t.Errorf("check %q missing or failed: %+v", want, res.Checks)
		}
	}
	if s := e.settings(); !s.LastDrillOK || s.LastDrillPartial || s.LastDrillAt.IsZero() {
		t.Fatalf("drill outcome not recorded: %+v", s)
	}
}

func TestRunDrill_PartialWithoutIdentity(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	e.backup()
	res, err := e.svc.RunDrill(context.Background())
	if err != nil || !res.OK || !res.Partial || !strings.Contains(res.Detail, "partial") {
		t.Fatalf("drill = %+v, err = %v", res, err)
	}
	if s := e.settings(); !s.LastDrillPartial {
		t.Fatalf("partial not recorded: %+v", s)
	}
	st, err := e.svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(st, "drill_partial") {
		t.Fatalf("warnings = %+v", st.Warnings)
	}
}

func TestRunDrill_FailurePaths(t *testing.T) {
	cases := []struct {
		name     string
		identity bool
		mangle   func(*testing.T, *drEnv, Manifest)
		want     error
	}{
		{"tampered ciphertext, checksum only", false, func(t *testing.T, e *drEnv, m Manifest) { flipBit(t, e, m.Key) }, ErrChecksumMismatch},
		{"tampered ciphertext, full drill", true, func(t *testing.T, e *drEnv, m Manifest) { flipBit(t, e, m.Key) }, ErrChecksumMismatch},
		{"manifest schema too new, checksum only", false, func(t *testing.T, e *drEnv, m Manifest) {
			m.SchemaVersion = 999999
			e.srv.Put(ManifestKey(m.Key), mustJSON(t, m), e.clock.Now())
		}, ErrNewerSchema},
		{"migration count disagrees with manifest", true, func(t *testing.T, e *drEnv, m Manifest) {
			m.MigrationsApplied++
			e.srv.Put(ManifestKey(m.Key), mustJSON(t, m), e.clock.Now())
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newDREnv(t)
			if tc.identity {
				e.svc.DrillIdentities = []age.Identity{e.id}
			}
			e.configure(nil)
			m := e.backup()
			tc.mangle(t, e, m)
			res, err := e.svc.RunDrill(context.Background())
			if err == nil || res.OK {
				t.Fatalf("drill should fail: %+v, %v", res, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			s := e.settings()
			if s.LastDrillOK || s.LastDrillAt.IsZero() || s.LastDrillDetail == "" {
				t.Fatalf("failure not recorded: %+v", s)
			}
			if p := e.svc.Problem(e.clock.Now()); !strings.Contains(p, "drill failed") {
				t.Fatalf("Problem = %q", p)
			}
		})
	}
}

func TestRunDrill_NoBackupsYet(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	if _, err := e.svc.RunDrill(context.Background()); !errors.Is(err, ErrNoBackups) {
		t.Fatalf("err = %v", err)
	}
}

func TestProblem_OverdueAndFailed(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	if p := e.svc.Problem(e.clock.Now()); p != "" {
		t.Fatalf("fresh install problem = %q", p)
	}
	e.backup()
	if _, err := e.svc.RunDrill(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p := e.svc.Problem(e.clock.Now()); p != "" {
		t.Fatalf("healthy problem = %q", p)
	}

	e.clock.Advance(26 * time.Hour)
	if p := e.svc.Problem(e.clock.Now()); p != "" {
		t.Fatalf("inside grace problem = %q", p)
	}
	e.clock.Advance(6 * time.Hour)
	if p := e.svc.Problem(e.clock.Now()); !strings.Contains(p, "backup is overdue") {
		t.Fatalf("overdue backup problem = %q", p)
	}
	e.backup()
	e.clock.Advance(7 * 24 * time.Hour)
	e.backup()
	if p := e.svc.Problem(e.clock.Now()); !strings.Contains(p, "drill is overdue") {
		t.Fatalf("overdue drill problem = %q", p)
	}

	e.srv.FailPuts = true
	e.clock.Advance(time.Minute)
	if _, err := e.svc.RunBackup(context.Background(), time.Time{}); err == nil {
		t.Fatal("expected failure")
	}
	if p := e.svc.Problem(e.clock.Now()); !strings.Contains(p, "backup failed") {
		t.Fatalf("failed backup problem = %q", p)
	}
}

func TestAlertSource(t *testing.T) {
	e := newDREnv(t)
	local := NewManager(e.db, e.dir)
	src := AlertSource{Local: local, Svc: e.svc, LocalScheduled: true}
	if _, ok, err := src.Newest(); err != nil || ok {
		t.Fatalf("no snapshots yet: ok=%v err=%v", ok, err)
	}
	e.configure(nil)
	e.backup()
	got, ok, err := src.Newest()
	if err != nil || !ok || !got.Equal(e.clock.Now()) {
		t.Fatalf("Newest = %v, %v, %v", got, ok, err)
	}
	if src.DRProblem(e.clock.Now()) != "" {
		t.Fatal("healthy setup reported a problem")
	}
}

func mustIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func hasWarning(st Status, code string) bool {
	for _, w := range st.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}
