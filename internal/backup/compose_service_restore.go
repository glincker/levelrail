package backup

import (
	"context"
	"errors"
	"fmt"
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
	container, err := c.Containers.ResolveComposeContainer(ctx, app, service)
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
	var plain io.Reader = dump
	if codec := composeRunCodec(run.ObjectKey); codec != "" {
		sealed, err := OpenSealed(dump, codec, c.Identities)
		if err != nil {
			return fmt.Errorf("open compose run %q: %w", run.ID, err)
		}
		defer func() { _ = sealed.Close() }()
		plain = sealed
	}
	if err := c.Restorer.Restore(ctx, engine, container, plain); err != nil {
		return fmt.Errorf("restore %s/%s from run %q: %w", app, service, run.ID, err)
	}
	return nil
}

// The restore commands mirror the compose dump commands: same env fallbacks, and the
// password travels in MYSQL_PWD from the container's own env, never on the command line.
// Postgres replays in one transaction after resetting the public schema, so a bad dump
// rolls back. MySQL and MariaDB dumps carry their own DROP TABLE and CREATE DATABASE.
var (
	composePostgresRestoreCmd = []string{"sh", "-c",
		`U="${POSTGRES_USER:-postgres}"; { echo "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"; cat; } | psql --no-password -v ON_ERROR_STOP=1 --single-transaction -U "$U" "${POSTGRES_DB:-$U}"`}
	composeMySQLRestoreCmd = []string{"sh", "-c",
		`export MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; exec mysql -uroot`}
	composeMariaDBRestoreCmd = []string{"sh", "-c",
		`export MYSQL_PWD="${MARIADB_ROOT_PASSWORD:-$MYSQL_ROOT_PASSWORD}"; exec mariadb -uroot`}
)

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
	Runtime docker.Runtime
}

// Restore implements Restorer.
func (c *ComposeContainerRestorer) Restore(ctx context.Context, engine, containerName string, dump io.Reader) error {
	cmd, err := ComposeRestoreCmd(engine)
	if err != nil {
		return err
	}
	rc, err := c.Runtime.ExecWithInput(ctx, containerName, cmd, dump)
	if err != nil {
		return fmt.Errorf("backup: restore %s compose service %q: %w", engine, containerName, err)
	}
	defer func() { _ = rc.Close() }()
	// Draining reaches EOF so a non-zero exit surfaces here.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("backup: restore %s compose service %q: %w", engine, containerName, err)
	}
	return nil
}
