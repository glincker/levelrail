package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrComposeRestoreRefused marks a restore rejected before anything was touched.
var ErrComposeRestoreRefused = errors.New("backup: compose restore refused")

// ComposeSafetyDumper takes the pre-restore dump; ComposeServiceRunner satisfies it.
type ComposeSafetyDumper interface {
	RunComposeServiceBackup(ctx context.Context, runID, app, service, engine, targetID string) error
}

// ComposeDestinationResolver resolves a backup target to a readable destination.
type ComposeDestinationResolver interface {
	ResolveDestination(ctx context.Context, targetID string) (Destination, error)
}

// ComposeServiceRestorer replays a succeeded compose dump into the running database container.
type ComposeServiceRestorer struct {
	Safety       ComposeSafetyDumper
	Destinations ComposeDestinationResolver
	Containers   ComposeContainerResolver
	Downloader   Downloader
	Restorer     Restorer
	// Identities decrypt sealed dumps; empty is valid for raw ones.
	Identities []age.Identity
}

// ComposeRestoreEngineSupported reports whether in-place restore covers engine.
func ComposeRestoreEngineSupported(engine string) bool {
	switch engine {
	case store.EnginePostgres, store.EngineMySQL, store.EngineMariaDB:
		return true
	}
	return false
}

// ComposeRestorePhrase is the exact text a caller must echo back to confirm.
func ComposeRestorePhrase(app, service string) string {
	return "restore " + app + "/" + service
}

// composeRunCodec derives the codec from the object key, since compose runs do not store it.
func composeRunCodec(objectKey string) string {
	switch {
	case strings.HasSuffix(objectKey, CodecSuffix(CodecZstdAge)):
		return CodecZstdAge
	case strings.HasSuffix(objectKey, CodecSuffix(CodecZstd)):
		return CodecZstd
	}
	return ""
}

// RunComposeServiceRestore takes a safety dump to the run's own target, aborts if it fails,
// then replaces the live database with the dump recorded by run. Nothing is touched when a
// check before the safety dump fails.
func (c *ComposeServiceRestorer) RunComposeServiceRestore(ctx context.Context, safetyRunID, app, service, engine string, run store.ComposeServiceBackupRun) error {
	if !ComposeRestoreEngineSupported(engine) {
		return fmt.Errorf("%w: engine %q is not supported", ErrComposeRestoreRefused, engine)
	}
	if run.Status != store.BackupStatusSucceeded || run.ObjectKey == "" {
		return fmt.Errorf("%w: run %q did not succeed", ErrComposeRestoreRefused, run.ID)
	}
	if run.App != app || run.Service != service {
		return fmt.Errorf("%w: run %q belongs to another service", ErrComposeRestoreRefused, run.ID)
	}
	ctx, container, err := resolveComposeWithNode(ctx, c.Containers, app, service)
	if err != nil {
		return fmt.Errorf("resolve container for %s/%s: %w", app, service, err)
	}
	dest, err := c.Destinations.ResolveDestination(ctx, run.TargetID)
	if err != nil {
		return err
	}
	// Verified before the safety dump so a corrupt object is refused without touching the database.
	if err := verifyObjectBeforeRestore(ctx, c.Downloader, dest, store.BackupHistory{ID: run.ID, ObjectKey: run.ObjectKey, ChecksumSHA256: run.Checksum, SizeBytes: run.SizeBytes}); err != nil {
		return err
	}
	if err := c.Safety.RunComposeServiceBackup(ctx, safetyRunID, app, service, engine, run.TargetID); err != nil {
		return fmt.Errorf("pre-restore safety dump failed, database left untouched: %w", err)
	}
	dump, err := c.Downloader.Download(ctx, dest, run.ObjectKey)
	if err != nil {
		return fmt.Errorf("download compose run %q: %w", run.ID, err)
	}
	defer func() { _ = dump.Close() }()
	// Hashing the stream actually replayed closes the gap between the earlier verify and this use.
	raw := &integrityReader{src: dump, h: sha256.New(), want: run.Checksum, wantSize: run.SizeBytes}
	var plain io.Reader = raw
	if codec := composeRunCodec(run.ObjectKey); codec != "" {
		sealed, err := OpenSealed(raw, codec, c.Identities)
		if err != nil {
			return fmt.Errorf("open compose run %q: %w", run.ID, err)
		}
		defer func() { _ = sealed.Close() }()
		plain = &drainOnEOF{src: sealed, rest: raw}
	}
	if err := c.Restorer.Restore(ctx, engine, container, plain); err != nil {
		return fmt.Errorf("restore %s/%s from run %q: %w", app, service, run.ID, err)
	}
	return nil
}

// integrityReader hashes everything read through it and fails the final read when the
// bytes differ from what was recorded at backup time.
type integrityReader struct {
	src      io.Reader
	h        hash.Hash
	n        int64
	want     string
	wantSize int64
}

func (r *integrityReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	_, _ = r.h.Write(p[:n])
	r.n += int64(n)
	if errors.Is(err, io.EOF) && r.want != "" {
		if hex.EncodeToString(r.h.Sum(nil)) != r.want || (r.wantSize != 0 && r.n != r.wantSize) {
			return n, errors.New("backup object changed since it was verified")
		}
	}
	return n, err
}

// drainOnEOF reads the raw stream to its end once the decoded one finishes, so the
// integrity check always sees the whole object before a commit is allowed.
type drainOnEOF struct {
	src  io.Reader
	rest io.Reader
}

func (d *drainOnEOF) Read(p []byte) (int, error) {
	n, err := d.src.Read(p)
	if errors.Is(err, io.EOF) {
		if _, derr := io.Copy(io.Discard, d.rest); derr != nil {
			return n, derr
		}
	}
	return n, err
}

// The restore commands mirror the compose dump commands: same env fallbacks, and the
// password travels in MYSQL_PWD from the container's own env, never on the command line.
// Postgres gets its transaction from Go (see guardedReader), so a stream that ends early
// is never committed. MySQL and MariaDB drop and recreate the one named database first so
// the result replaces it; a server-wide dump is refused because it would rewrite the
// system schema and the root credentials.
const composeMySQLReplace = `BT=$(printf '\140'); [ -n "$D" ] || { echo "restore needs MYSQL_DATABASE or MARIADB_DATABASE on the service; server-wide dumps are not restored in place" >&2; exit 3; }; ` +
	`%[1]s -uroot -e "DROP DATABASE IF EXISTS ${BT}${D}${BT}; CREATE DATABASE ${BT}${D}${BT}" || exit 1; exec %[1]s -uroot`

var (
	composePostgresRestoreCmd = []string{"sh", "-c",
		`U="${POSTGRES_USER:-postgres}"; exec psql --no-password -v ON_ERROR_STOP=1 -U "$U" "${POSTGRES_DB:-$U}"`}
	composeMySQLRestoreCmd = []string{"sh", "-c",
		`export MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; D="$MYSQL_DATABASE"; ` + fmt.Sprintf(composeMySQLReplace, "mysql")}
	composeMariaDBRestoreCmd = []string{"sh", "-c",
		`export MYSQL_PWD="${MARIADB_ROOT_PASSWORD:-$MYSQL_ROOT_PASSWORD}"; D="${MARIADB_DATABASE:-$MYSQL_DATABASE}"; ` + fmt.Sprintf(composeMySQLReplace, "mariadb")}
)

// composePostgresPrelude opens the transaction and clears every user schema (not system or
// extension-owned ones) so the replay lands on an empty scope; %I quotes each name.
// COMMIT is only sent by guardedReader after a clean end of the dump.
const composePostgresPrelude = `BEGIN;
DO $lr_reset$
DECLARE s name;
BEGIN
  FOR s IN
    SELECT n.nspname FROM pg_namespace n
    WHERE n.nspname <> 'information_schema' AND n.nspname !~ '^pg_'
      AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_namespace'::regclass AND d.objid = n.oid AND d.deptype = 'e')
  LOOP
    EXECUTE format('DROP SCHEMA %I CASCADE', s);
  END LOOP;
  CREATE SCHEMA public;
END
$lr_reset$;
`

// guardedReader frames the dump with an optional prelude and suffix. The suffix is only
// emitted on a clean EOF, and the first real read error is remembered so a truncated
// stream cannot look like a successful restore.
type guardedReader struct {
	src     io.Reader
	prelude string
	suffix  string
	err     error
	done    bool
}

func (g *guardedReader) Read(p []byte) (int, error) {
	if g.prelude != "" {
		n := copy(p, g.prelude)
		g.prelude = g.prelude[n:]
		return n, nil
	}
	if g.done {
		if g.suffix != "" {
			n := copy(p, g.suffix)
			g.suffix = g.suffix[n:]
			return n, nil
		}
		return 0, io.EOF
	}
	n, err := g.src.Read(p)
	if err == nil {
		return n, nil
	}
	if errors.Is(err, io.EOF) {
		g.done = true
		return n, nil
	}
	g.err = err
	return n, err
}

// ComposeRestoreCmd returns the in-container restore command for a compose database service.
func ComposeRestoreCmd(engine string) ([]string, error) {
	switch engine {
	case store.EnginePostgres:
		return composePostgresRestoreCmd, nil
	case store.EngineMySQL:
		return composeMySQLRestoreCmd, nil
	case store.EngineMariaDB:
		return composeMariaDBRestoreCmd, nil
	}
	return nil, fmt.Errorf("backup: compose service restore: engine %q is not supported", engine)
}

// ComposeContainerRestorer is the Restorer for compose database services, the counterpart of ComposeServiceDumper.
type ComposeContainerRestorer struct {
	Runtime  docker.Runtime
	Runtimes func(nodeID string) (docker.Runtime, error)
}

// Restore implements Restorer.
func (c *ComposeContainerRestorer) Restore(ctx context.Context, engine, containerName string, dump io.Reader) error {
	cmd, err := ComposeRestoreCmd(engine)
	if err != nil {
		return err
	}
	g := &guardedReader{src: dump}
	if engine == store.EnginePostgres {
		g.prelude, g.suffix = composePostgresPrelude, "\nCOMMIT;\n"
	}
	rt, err := composeRuntime(ctx, c.Runtime, c.Runtimes)
	if err != nil {
		return fmt.Errorf("backup: restore %s compose service %q: %w", engine, containerName, err)
	}
	rc, err := rt.ExecWithInput(ctx, containerName, cmd, g)
	if err != nil {
		return fmt.Errorf("backup: restore %s compose service %q: %w", engine, containerName, err)
	}
	defer func() { _ = rc.Close() }()
	// Draining reaches EOF so a non-zero exit surfaces here.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("backup: restore %s compose service %q: %w", engine, containerName, err)
	}
	if g.err != nil {
		return fmt.Errorf("backup: restore %s compose service %q: dump stream failed, not committed: %w", engine, containerName, g.err)
	}
	return nil
}
