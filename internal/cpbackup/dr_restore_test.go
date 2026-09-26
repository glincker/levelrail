package cpbackup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/store"
)

func restoreFrom(t *testing.T, e *drEnv, key, live string, mut func(*RestoreOptions)) (RestoreReport, error) {
	t.Helper()
	opts := RestoreOptions{LivePath: live, Identities: []age.Identity{e.id}}
	if mut != nil {
		mut(&opts)
	}
	return Restore(context.Background(), NewBucketSource(newBucket(t, e.srv), key), opts)
}

func flipBit(t *testing.T, e *drEnv, key string) {
	t.Helper()
	data, ok := e.srv.Data(key)
	if !ok {
		t.Fatalf("no object %q", key)
	}
	mangled := bytes.Clone(data)
	mangled[len(mangled)/2] ^= 0x01
	e.srv.Put(key, mangled, e.clock.Now())
}

func TestRestore_TamperAndWrongIdentity(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	live := filepath.Join(t.TempDir(), "levelrail.db")

	other, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreFrom(t, e, m.Key, live, func(o *RestoreOptions) { o.Identities = []age.Identity{other} }); !errors.Is(err, ErrWrongIdentity) {
		t.Fatalf("wrong identity err = %v", err)
	}
	if _, err := restoreFrom(t, e, m.Key, live, func(o *RestoreOptions) { o.Identities = nil }); err == nil {
		t.Fatal("restore without an identity must fail")
	}

	flipBit(t, e, m.Key)
	if _, err := restoreFrom(t, e, m.Key, live, nil); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("bit flip err = %v, want checksum mismatch", err)
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatal("a failed restore created the live database")
	}
}

func TestRestore_TamperWithMatchingManifestStillFailsDecrypt(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	flipBit(t, e, m.Key)
	data, _ := e.srv.Data(m.Key)
	forged := m
	_, forged.SHA256 = sumBytes(data)
	e.srv.Put(ManifestKey(m.Key), mustJSON(t, forged), e.clock.Now())
	live := filepath.Join(t.TempDir(), "levelrail.db")
	if _, err := restoreFrom(t, e, m.Key, live, nil); err == nil || errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("authenticated encryption should reject the forged file, got %v", err)
	}
}

func TestRestore_ManifestProblems(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	live := filepath.Join(t.TempDir(), "levelrail.db")

	cases := []struct {
		name string
		mut  func(*Manifest)
		want error
	}{
		{"sha mismatch", func(x *Manifest) { x.SHA256 = "00" }, ErrChecksumMismatch},
		{"size mismatch", func(x *Manifest) { x.SizeBytes++ }, ErrChecksumMismatch},
		{"plain sha mismatch", func(x *Manifest) { x.PlainSHA256 = "00" }, ErrChecksumMismatch},
		{"missing checksum", func(x *Manifest) { x.SHA256 = "" }, ErrBadManifest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := m
			tc.mut(&bad)
			e.srv.Put(ManifestKey(m.Key), mustJSON(t, bad), e.clock.Now())
			if _, err := restoreFrom(t, e, m.Key, live, nil); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	e.srv.Put(ManifestKey(m.Key), []byte("not json"), e.clock.Now())
	if _, err := restoreFrom(t, e, m.Key, live, nil); !errors.Is(err, ErrBadManifest) {
		t.Fatalf("garbage manifest err = %v", err)
	}
}

func TestRestore_DryRunLeavesLiveUntouched(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	live := filepath.Join(t.TempDir(), "levelrail.db")
	writeFile(t, live, "precious")

	rep, err := restoreFrom(t, e, m.Key, live, func(o *RestoreOptions) { o.DryRun = true })
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !rep.DryRun || len(rep.Checks) < 4 {
		t.Fatalf("report = %+v", rep)
	}
	if got := readFile(t, live); got != "precious" {
		t.Fatal("dry run changed the live database")
	}
	assertNoLeftovers(t, filepath.Dir(live), "levelrail.db")
}

func TestRestore_CrashBetweenTempWriteAndRenameKeepsOldDatabase(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	for _, stage := range []string{"temp-written", "pre-restore-kept"} {
		t.Run(stage, func(t *testing.T) {
			live := filepath.Join(t.TempDir(), "levelrail.db")
			writeFile(t, live, "old database")
			crash := errors.New("simulated crash")
			_, err := restoreFrom(t, e, m.Key, live, func(o *RestoreOptions) {
				o.hook = func(s string) error {
					if s == stage {
						return crash
					}
					return nil
				}
			})
			if !errors.Is(err, crash) {
				t.Fatalf("err = %v", err)
			}
			if got := readFile(t, live); got != "old database" {
				t.Fatalf("live database = %q after a crash, want the old one", got)
			}
			assertNoLeftovers(t, filepath.Dir(live), "levelrail.db", "levelrail.db.pre-restore")
		})
	}
}

func TestRestore_SwapKeepsPreRestoreCopy(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	live := filepath.Join(t.TempDir(), "levelrail.db")
	writeFile(t, live, "old one")
	writeFile(t, live+"-wal", "old wal")
	writeFile(t, live+".pre-restore", "older still")

	rep, err := restoreFrom(t, e, m.Key, live, func(o *RestoreOptions) { o.Now = func() time.Time { return e.clock.Now() } })
	if err != nil {
		t.Fatal(err)
	}
	if rep.PreRestore != live+".pre-restore" || readFile(t, live+".pre-restore") != "old one" || readFile(t, live+".pre-restore-wal") != "old wal" {
		t.Fatalf("pre-restore copy wrong: %+v", rep)
	}
	if readFile(t, live+".pre-restore-20260925T020000Z") != "older still" {
		t.Fatal("earlier pre-restore copy was overwritten")
	}
	if _, err := os.Stat(live + "-wal"); !os.IsNotExist(err) {
		t.Fatal("stale wal left beside the restored database")
	}
	if _, err := store.InspectSnapshot(context.Background(), live); err != nil {
		t.Fatalf("restored database invalid: %v", err)
	}
}

func TestRestore_NewerSchemaRefused(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()
	// Rebuild the backup from a database claiming a future schema version.
	plain := filepath.Join(t.TempDir(), "future.db")
	if _, _, err := e.db.SnapshotTo(context.Background(), plain); err != nil {
		t.Fatal(err)
	}
	execSQL(t, plain, `INSERT INTO schema_migrations (version, name) VALUES (999999, 'from the future')`)
	recips, err := ParseRecipients([]string{e.id.Recipient().String()})
	if err != nil {
		t.Fatal(err)
	}
	cipher := filepath.Join(t.TempDir(), "future.db.age")
	sl, err := seal(plain, cipher, recips)
	if err != nil {
		t.Fatal(err)
	}
	future := m
	future.SHA256, future.SizeBytes, future.PlainSHA256, future.PlainSizeBytes = sl.SHA256, sl.Size, sl.PlainSHA256, sl.PlainSize
	writeFile(t, ManifestKey(cipher), string(mustJSON(t, future)))

	live := filepath.Join(t.TempDir(), "levelrail.db")
	writeFile(t, live, "current")
	_, err = Restore(context.Background(), NewFileSource(cipher), RestoreOptions{LivePath: live, Identities: []age.Identity{e.id}})
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("err = %v, want ErrNewerSchema", err)
	}
	if readFile(t, live) != "current" {
		t.Fatal("live database changed")
	}
}

func TestRestore_InstallIDGuard(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	m := e.backup()

	live := filepath.Join(t.TempDir(), "levelrail.db")
	if _, err := restoreFrom(t, e, m.Key, live, nil); err != nil {
		t.Fatal(err)
	}
	execSQL(t, live, `UPDATE control_plane_dr_settings SET install_id = 'inst-someone-else' WHERE id = 1`)

	if _, err := restoreFrom(t, e, m.Key, live, nil); !errors.Is(err, ErrInstallMismatch) {
		t.Fatalf("err = %v, want ErrInstallMismatch", err)
	}
	if _, err := restoreFrom(t, e, m.Key, live, func(o *RestoreOptions) { o.ForceInstallID = true }); err != nil {
		t.Fatalf("forced restore: %v", err)
	}
	if readInstallID(context.Background(), live) != m.InstallID {
		t.Fatal("forced restore did not install the backup")
	}
}

func TestLatestKey(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	first := e.backup()
	e.clock.Advance(time.Hour)
	second := e.backup()
	e.srv.Put(DataKey(first.InstallID, e.clock.Now().Add(time.Hour)), []byte("orphan with no manifest"), e.clock.Now())

	got, err := LatestKey(context.Background(), newBucket(t, e.srv), installPrefix(first.InstallID))
	if err != nil || got != second.Key {
		t.Fatalf("LatestKey = %q, %v; want %q", got, err, second.Key)
	}
	if _, err := LatestKey(context.Background(), newBucket(t, e.srv), "cp-backups/nobody/"); !errors.Is(err, ErrNoBackups) {
		t.Fatalf("empty prefix err = %v", err)
	}
}
