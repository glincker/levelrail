package mailbackup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

type fakeRunner struct {
	host, cont string
	version    string
	verifyErr  error
	calls      []string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch args[0] {
	case "version":
		return f.version + " go1.24 linux/amd64", nil
	case "backup":
		d := filepath.Join(f.host, strings.TrimPrefix(args[1], f.cont))
		if err := os.MkdirAll(filepath.Join(d, "data", "accounts"), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(d, "data", "accounts", "a.db"), []byte("db-bytes"), 0o600); err != nil {
			return "", err
		}
		return "", os.WriteFile(filepath.Join(d, "mox.conf"), []byte("conf"), 0o600)
	case "verifydata":
		return "", f.verifyErr
	}
	return "", nil
}

type env struct {
	id    *age.X25519Identity
	run   *fakeRunner
	sink  *LocalSink
	stage string
	now   time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	return &env{
		id: id, stage: stage,
		run:  &fakeRunner{host: stage, cont: "/mox/staging", version: "v0.0.15"},
		sink: &LocalSink{Dir: t.TempDir()},
		now:  time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC),
	}
}

func (e *env) backup(t *testing.T) (Manifest, error) {
	t.Helper()
	e.now = e.now.Add(24 * time.Hour)
	return Backup(context.Background(), BackupSpec{
		Runner: e.run, Sink: e.sink, StagingHost: e.stage, StagingContainer: "/mox/staging",
		Recipients: []age.Recipient{e.id.Recipient()}, Now: func() time.Time { return e.now },
	})
}

func (e *env) restoreSpec(id, dest string) RestoreSpec {
	return RestoreSpec{Runner: e.run, Sink: e.sink, ID: id, DestDir: dest, Identities: []age.Identity{e.id}}
}

func stagingEmpty(t *testing.T, dir string) {
	t.Helper()
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("staging not cleaned: %v", ents)
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	e := newEnv(t)
	m, err := e.backup(t)
	if err != nil {
		t.Fatal(err)
	}
	if m.Files != 2 || m.MoxVersion != "v0.0.15" || m.SHA256 == "" {
		t.Fatalf("manifest %+v", m)
	}
	stagingEmpty(t, e.stage)

	dry, err := Restore(context.Background(), e.restoreSpec(m.ID, filepath.Join(t.TempDir(), "out")), true)
	if err != nil || dry.Files != 2 || !dry.DryRun {
		t.Fatalf("dry %+v %v", dry, err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if _, err := Restore(context.Background(), e.restoreSpec(m.ID, dest), false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "data", "accounts", "a.db")) //nolint:gosec // test temp dir
	if err != nil || string(got) != "db-bytes" {
		t.Fatalf("restored %q %v", got, err)
	}
}

func TestBackupCleansStagingOnFailure(t *testing.T) {
	e := newEnv(t)
	e.run.verifyErr = errors.New("verify failed")
	if _, err := e.backup(t); err == nil {
		t.Fatal("want error")
	}
	stagingEmpty(t, e.stage)
	if names, _ := e.sink.List(context.Background()); len(names) != 0 {
		t.Fatalf("nothing must be uploaded after a failed verify, got %v", names)
	}
}

func TestBackupExceedsCapKeepsExisting(t *testing.T) {
	e := newEnv(t)
	first, err := e.backup(t)
	if err != nil {
		t.Fatal(err)
	}
	e.sink.CapBytes = 10
	_, err = e.backup(t)
	var ce *CapError
	if !errors.As(err, &ce) {
		t.Fatalf("want CapError, got %v", err)
	}
	stagingEmpty(t, e.stage)
	if _, err := e.sink.Open(context.Background(), manifestName(first.ID)); err != nil {
		t.Fatalf("existing backup must survive: %v", err)
	}
}

func TestRestoreRefusals(t *testing.T) {
	cases := []struct {
		name   string
		target string
		tamper bool
		want   any
	}{
		{"downgrade", "v0.0.14", false, new(*DowngradeError)},
		{"checksum", "v0.0.15", true, new(*ChecksumError)},
		{"newer target ok", "v0.1.0", false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			m, err := e.backup(t)
			if err != nil {
				t.Fatal(err)
			}
			e.run.version = c.target
			if c.tamper {
				p := filepath.Join(e.sink.Dir, m.ArchiveName)
				raw, _ := os.ReadFile(p) //nolint:gosec // test temp dir
				raw[len(raw)-1] ^= 0xff
				_ = os.WriteFile(p, raw, 0o600) //nolint:gosec // test path in a temp dir
			}
			dest := filepath.Join(t.TempDir(), "out")
			_, err = Restore(context.Background(), e.restoreSpec(m.ID, dest), false)
			switch c.want.(type) {
			case nil:
				if err != nil {
					t.Fatal(err)
				}
			case **DowngradeError:
				var d *DowngradeError
				if !errors.As(err, &d) {
					t.Fatalf("got %v", err)
				}
			case **ChecksumError:
				var d *ChecksumError
				if !errors.As(err, &d) {
					t.Fatalf("got %v", err)
				}
			}
			if c.want != nil {
				if _, serr := os.Stat(dest); serr == nil {
					t.Fatal("refused restore must not create the target")
				}
				if _, serr := os.Stat(dest + ".restoring"); serr == nil {
					t.Fatal("work dir leaked")
				}
			}
		})
	}
}

func seed(t *testing.T, s *LocalSink, id string, size int, created time.Time, verified bool) {
	t.Helper()
	ctx := context.Background()
	if err := s.Put(ctx, archiveName(id), bytes.NewReader(make([]byte, size))); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Manifest{ID: id, CreatedAt: created, Verified: verified})
	if err := s.Put(ctx, manifestName(id), bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestPruneOrderAndProtection(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		cap      int64
		verified [3]bool
		want     []string
		wantCap  bool
	}{
		{"oldest first", 2300, [3]bool{true, true, true}, []string{"a"}, false},
		{"two removed", 1300, [3]bool{true, true, true}, []string{"a", "b"}, false},
		{"newest verified kept over newer unverified", 1300, [3]bool{true, true, false}, []string{"a", "c"}, false},
		{"newest verified alone too big", 500, [3]bool{true, true, true}, []string{"a", "b"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &LocalSink{Dir: t.TempDir()}
			for i, id := range []string{"a", "b", "c"} {
				seed(t, s, id, 1000, base.Add(time.Duration(i)*time.Hour), c.verified[i])
			}
			s.CapBytes = c.cap
			removed, err := s.Prune(context.Background())
			var ce *CapError
			if got := errors.As(err, &ce); got != c.wantCap {
				t.Fatalf("cap error %v, err %v", got, err)
			}
			if strings.Join(removed, ",") != strings.Join(c.want, ",") {
				t.Fatalf("removed %v want %v", removed, c.want)
			}
			newest := "b"
			if c.verified[2] {
				newest = "c"
			}
			if _, err := s.Open(context.Background(), archiveName(newest)); err != nil {
				t.Fatalf("newest verified %s removed: %v", newest, err)
			}
		})
	}
}

func TestCapFromEnv(t *testing.T) {
	get := func(v string) func(string) string { return func(string) string { return v } }
	if n, err := CapFromEnv(get("")); err != nil || n != DefaultLocalCapMB<<20 {
		t.Fatalf("default %d %v", n, err)
	}
	if n, err := CapFromEnv(get("5")); err != nil || n != 5<<20 {
		t.Fatalf("set %d %v", n, err)
	}
	for _, bad := range []string{"0", "-1", "x"} {
		if _, err := CapFromEnv(get(bad)); err == nil {
			t.Fatalf("%q must fail", bad)
		}
	}
}
