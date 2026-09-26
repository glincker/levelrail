package cpbackup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Errors a restore can end with.
var (
	ErrChecksumMismatch = errors.New("downloaded backup does not match its manifest checksum")
	ErrNewerSchema      = errors.New("backup schema is newer than this binary supports")
	ErrInstallMismatch  = errors.New("backup belongs to a different install")
)

// Source is where a restore reads an encrypted backup from.
type Source interface {
	Describe() string
	ReadManifest(ctx context.Context) (Manifest, error)
	Fetch(ctx context.Context, dst string, maxBytes int64) error
}

type bucketSource struct {
	b   Bucket
	key string
}

// NewBucketSource reads the backup at key from a bucket.
func NewBucketSource(b Bucket, key string) Source { return bucketSource{b: b, key: key} }

func (s bucketSource) Describe() string { return s.key }

func (s bucketSource) ReadManifest(ctx context.Context) (Manifest, error) {
	return FetchManifest(ctx, s.b, s.key)
}

func (s bucketSource) Fetch(ctx context.Context, dst string, maxBytes int64) error {
	rc, _, err := s.b.Get(ctx, s.key)
	if err != nil {
		return fmt.Errorf("download backup: %w", err)
	}
	defer func() { _ = rc.Close() }()
	return writeLimited(dst, rc, maxBytes)
}

type fileSource struct{ path string }

// NewFileSource reads an encrypted backup from disk. Its manifest is the
// sibling file with the .json suffix.
func NewFileSource(path string) Source { return fileSource{path: path} }

func (s fileSource) Describe() string { return s.path }

func (s fileSource) ReadManifest(_ context.Context) (Manifest, error) {
	data, err := os.ReadFile(ManifestKey(s.path)) //nolint:gosec // operator-supplied path
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest beside the backup file: %w", err)
	}
	return ParseManifest(data)
}

func (s fileSource) Fetch(_ context.Context, dst string, maxBytes int64) error {
	f, err := os.Open(s.path) //nolint:gosec // operator-supplied path
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return writeLimited(dst, f, maxBytes)
}

func writeLimited(dst string, r io.Reader, maxBytes int64) error {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // path built by this package
	if err != nil {
		return fmt.Errorf("create download file: %w", err)
	}
	n, err := io.Copy(out, io.LimitReader(r, maxBytes+1))
	if err == nil && n > maxBytes {
		err = ErrChecksumMismatch
	}
	if err != nil {
		_ = out.Close()
		return fmt.Errorf("write download: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync download: %w", err)
	}
	return out.Close()
}

// LatestKey returns the newest complete backup under prefix.
func LatestKey(ctx context.Context, b Bucket, prefix string) (string, error) {
	remotes, err := listRemotes(ctx, b, prefix)
	if err != nil {
		return "", err
	}
	for _, r := range remotes {
		if r.Complete {
			return r.Key, nil
		}
	}
	return "", ErrNoBackups
}

// RestoreOptions control one restore.
type RestoreOptions struct {
	// LivePath is the control plane database file to replace.
	LivePath   string
	Identities []age.Identity
	// DryRun verifies everything and leaves the live database untouched.
	DryRun bool
	// ForceInstallID accepts a backup taken by a different install.
	ForceInstallID bool
	Now            func() time.Time

	hook func(stage string) error
}

// RestoreReport is what a restore checked and did.
type RestoreReport struct {
	Source        string   `json:"source"`
	Manifest      Manifest `json:"manifest"`
	SchemaVersion int      `json:"schema_version"`
	Checks        []Check  `json:"checks"`
	DryRun        bool     `json:"dry_run"`
	PreRestore    string   `json:"pre_restore,omitempty"`
}

// Restore downloads, verifies and decrypts a backup, then swaps it in
// atomically. The live database file is replaced by one rename, so an
// interruption at any earlier point leaves the old database untouched.
func Restore(ctx context.Context, src Source, opts RestoreOptions) (RestoreReport, error) {
	rep := RestoreReport{Source: src.Describe(), DryRun: opts.DryRun}
	m, err := src.ReadManifest(ctx)
	if err != nil {
		return rep, err
	}
	rep.Manifest = m
	rep.Checks = append(rep.Checks, Check{Name: "manifest", OK: true, Detail: fmt.Sprintf("install %s, schema %d, binary %s", m.InstallID, m.SchemaVersion, m.BinaryVersion)})

	dir := filepath.Dir(opts.LivePath)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // operator-controlled data dir
		return rep, fmt.Errorf("create data dir: %w", err)
	}
	work, err := os.MkdirTemp(dir, ".restore-work-*")
	if err != nil {
		return rep, fmt.Errorf("create work dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	restored, err := fetchAndVerify(ctx, src, m, work, opts.Identities, &rep)
	if err != nil {
		return rep, err
	}
	version, err := store.InspectSnapshot(ctx, restored)
	if err != nil {
		return rep, fmt.Errorf("restored database failed integrity check: %w", err)
	}
	rep.SchemaVersion = version
	rep.Checks = append(rep.Checks, Check{Name: "integrity", OK: true, Detail: "integrity_check ok"})
	latest, err := store.MaxSchemaVersion()
	if err != nil {
		return rep, err
	}
	if version > latest {
		return rep, fmt.Errorf("%w: backup is at version %d, this binary supports up to %d", ErrNewerSchema, version, latest)
	}
	rep.Checks = append(rep.Checks, Check{Name: "schema_version", OK: true, Detail: fmt.Sprintf("version %d, this binary supports up to %d", version, latest)})
	if err := checkInstall(ctx, m, restored, opts); err != nil {
		return rep, err
	}
	rep.Checks = append(rep.Checks, Check{Name: "install_id", OK: true, Detail: "install id matches"})
	if opts.DryRun {
		return rep, nil
	}
	pre, err := swapIn(restored, opts)
	rep.PreRestore = pre
	return rep, err
}

func fetchAndVerify(ctx context.Context, src Source, m Manifest, work string, ids []age.Identity, rep *RestoreReport) (string, error) {
	cipherPath := filepath.Join(work, "backup.db.age")
	if err := src.Fetch(ctx, cipherPath, m.SizeBytes); err != nil {
		return "", err
	}
	size, sum, err := store.FileSHA256(cipherPath)
	if err != nil {
		return "", err
	}
	if size != m.SizeBytes || sum != m.SHA256 {
		return "", fmt.Errorf("%w: expected %s (%d bytes), got %s (%d bytes)", ErrChecksumMismatch, m.SHA256, m.SizeBytes, sum, size)
	}
	rep.Checks = append(rep.Checks, Check{Name: "checksum", OK: true, Detail: "sha256 matches the manifest"})
	if len(ids) == 0 {
		return "", errors.New("an identity file is required to decrypt the backup")
	}
	restored := filepath.Join(work, "restored.db")
	got, err := open(cipherPath, restored, ids, m.PlainSizeBytes)
	if err != nil {
		return "", err
	}
	if got.PlainSHA256 != m.PlainSHA256 || got.PlainSize != m.PlainSizeBytes {
		return "", fmt.Errorf("%w: decrypted database differs from the manifest", ErrChecksumMismatch)
	}
	rep.Checks = append(rep.Checks, Check{Name: "decrypt", OK: true, Detail: "decrypted and plaintext checksum matches"})
	return restored, nil
}

func checkInstall(ctx context.Context, m Manifest, restored string, opts RestoreOptions) error {
	if opts.ForceInstallID {
		return nil
	}
	if inner := readInstallID(ctx, restored); inner != "" && inner != m.InstallID {
		return fmt.Errorf("%w: manifest says %s but the database says %s; use --force-install-id to override", ErrInstallMismatch, m.InstallID, inner)
	}
	if local := readInstallID(ctx, opts.LivePath); local != "" && local != m.InstallID {
		return fmt.Errorf("%w: this server is %s but the backup is from %s; use --force-install-id to override", ErrInstallMismatch, local, m.InstallID)
	}
	return nil
}

func readInstallID(ctx context.Context, path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return ""
	}
	defer func() { _ = db.Close() }()
	var id string
	if err := db.QueryRowContext(ctx, `SELECT install_id FROM control_plane_dr_settings WHERE id = 1`).Scan(&id); err != nil {
		return ""
	}
	return id
}

// swapIn keeps the current database as <live>.pre-restore, then renames the
// restored file over it. Hard links keep the live name in place until the
// final rename, so there is no moment without a database.
func swapIn(restored string, opts RestoreOptions) (string, error) {
	live := opts.LivePath
	dir := filepath.Dir(live)
	stage := func(name string) error {
		if opts.hook == nil {
			return nil
		}
		return opts.hook(name)
	}
	staged, err := os.CreateTemp(dir, ".restore-*.tmp")
	if err != nil {
		return "", fmt.Errorf("stage restored database: %w", err)
	}
	stagedPath := staged.Name()
	_ = staged.Close()
	defer func() { _ = os.Remove(stagedPath) }()
	if err := copyFileSync(restored, stagedPath); err != nil {
		return "", err
	}
	if err := stage("temp-written"); err != nil {
		return "", err
	}

	pre := live + ".pre-restore"
	if _, err := os.Stat(live); err == nil {
		if _, err := os.Stat(pre); err == nil {
			older := pre + "-" + nowUTC(opts).Format(nameLayout)
			if err := os.Rename(pre, older); err != nil {
				return "", fmt.Errorf("age earlier pre-restore copy: %w", err)
			}
		}
		if err := preserve(live, pre); err != nil {
			return "", err
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, err := os.Stat(live + suffix); err == nil {
				if err := preserve(live+suffix, pre+suffix); err != nil {
					return "", err
				}
			}
		}
	} else {
		pre = ""
	}
	if err := stage("pre-restore-kept"); err != nil {
		return pre, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(live + suffix); err != nil && !os.IsNotExist(err) {
			return pre, fmt.Errorf("remove stale %s file: %w", suffix, err)
		}
	}
	if err := os.Rename(stagedPath, live); err != nil {
		return pre, fmt.Errorf("put restored database in place: %w", err)
	}
	syncDir(dir)
	return pre, nil
}

func nowUTC(opts RestoreOptions) time.Time {
	if opts.Now != nil {
		return opts.Now().UTC()
	}
	return time.Now().UTC()
}

func preserve(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	if err := copyFileSync(src, dst); err != nil {
		return fmt.Errorf("keep %s: %w", filepath.Base(dst), err)
	}
	return nil
}

func copyFileSync(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // path built by this package
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // path built by this package
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil { //nolint:gosec // operator-controlled data dir
		_ = d.Sync()
		_ = d.Close()
	}
}
