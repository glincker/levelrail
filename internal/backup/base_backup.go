package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// postgresWALArchivePath duplicates internal/reconcile/database's own
// identical constant (see that package's own doc comment on why this
// codebase duplicates small, stable formats rather than importing).
const postgresWALArchivePath = "/var/lib/postgresql/wal_archive"

// BaseBackuper produces a physical base backup as a tar stream, for a
// single PITR-enabled Postgres database container already running under
// containerName: the physical counterpart of Dumper (dump.go), which
// produces a logical dump.
type BaseBackuper interface {
	BaseBackup(ctx context.Context, containerName string) (io.ReadCloser, error)
}

// ContainerBaseBackuper is the real BaseBackuper: it runs pg_basebackup
// inside the already-running container via docker.Runtime.Exec, the same
// "no separate copy of the tool, no exposed port" reasoning
// ContainerDumper's own doc comment gives.
type ContainerBaseBackuper struct {
	Runtime Runtime
}

// Runtime is the narrow docker.Runtime surface (Exec/ExecWithInput)
// this file's functions and ContainerPITRRestorer need.
type Runtime interface {
	Exec(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error)
	ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error)
}

// BaseBackup implements BaseBackuper. -X none skips pg_basebackup's own
// WAL streaming: the database already archives WAL continuously via
// archive_command, so a restore replays from the archive, not from
// anything bundled into the backup itself. See ADR 016.
func (b *ContainerBaseBackuper) BaseBackup(ctx context.Context, containerName string) (io.ReadCloser, error) {
	rc, err := b.Runtime.Exec(ctx, containerName, pgBaseBackupCmd)
	if err != nil {
		return nil, fmt.Errorf("backup: base backup container %q: %w", containerName, err)
	}
	return rc, nil
}

// pgBaseBackupCmd authenticates the same way postgresDumpCmd does: local
// socket, trust auth, $POSTGRES_USER for both role and target database.
// --checkpoint=fast forces an immediate checkpoint rather than waiting
// for the next scheduled one, so a triggered base backup starts promptly
// instead of stalling for up to checkpoint_timeout.
var pgBaseBackupCmd = []string{"sh", "-c", `exec pg_basebackup -U "$POSTGRES_USER" -D - -Ft -X none --checkpoint=fast`}

// StartLSN best-effort queries the current WAL insert location for
// BaseBackupHistory.LSN's diagnostics-only purpose. Failures are
// swallowed: losing this one diagnostic string must never fail an
// otherwise-successful base backup.
func StartLSN(ctx context.Context, rt Runtime, containerName string) string {
	rc, err := rt.Exec(ctx, containerName, pgCurrentWALLSNCmd)
	if err != nil {
		return ""
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

var pgCurrentWALLSNCmd = []string{"sh", "-c", `exec psql --no-password -U "$POSTGRES_USER" -Atq -c "SELECT pg_current_wal_lsn()"`}

// RecoverableWindowEnd returns the latest timestamp a PITR restore of
// containerName can safely target: forces a WAL switch and waits for
// archive_command to actually land it, rather than trusting
// archive_timeout's own lag, so the bound is provably exact. A target
// beyond what's genuinely archived would hang recovery rather than fail
// it (ValidatePITRTarget).
func RecoverableWindowEnd(ctx context.Context, rt Runtime, containerName string) (time.Time, error) {
	now, walFile, err := currentWALFile(ctx, rt, containerName)
	if err != nil {
		return time.Time{}, err
	}

	if err := switchWAL(ctx, rt, containerName); err != nil {
		return time.Time{}, err
	}

	deadline := time.Now().Add(walArchivePollTimeout)
	for time.Now().Before(deadline) {
		if archived(ctx, rt, containerName, walFile) {
			return now, nil
		}
		time.Sleep(walArchivePollInterval)
	}
	return time.Time{}, fmt.Errorf("backup: wal segment %q was not archived within %s", walFile, walArchivePollTimeout)
}

const (
	walArchivePollTimeout  = 15 * time.Second
	walArchivePollInterval = 300 * time.Millisecond
)

// currentWALFile returns the server's current time and the WAL segment
// filename that time falls within, both from the same SQL round trip so
// they describe the identical instant.
func currentWALFile(ctx context.Context, rt Runtime, containerName string) (time.Time, string, error) {
	rc, err := rt.Exec(ctx, containerName, pgNowAndWALFileCmd)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("backup: query current wal file: %w", err)
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return time.Time{}, "", fmt.Errorf("backup: query current wal file: %w", err)
	}
	fields := strings.Fields(buf.String())
	if len(fields) != 2 {
		return time.Time{}, "", fmt.Errorf("backup: query current wal file: unexpected output %q", buf.String())
	}
	epochSeconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("backup: query current wal file: parse epoch %q: %w", fields[0], err)
	}
	now := time.UnixMicro(int64(epochSeconds * 1e6)).UTC()
	return now, fields[1], nil
}

// pgNowAndWALFileCmd returns the server's current time as a Unix epoch
// (fractional seconds) rather than a formatted timestamp string: a plain
// number sidesteps locale/format-string quoting entirely inside an
// already-quoted shell -c command, at the cost of nothing since Go parses
// it straight into a time.Time.
var pgNowAndWALFileCmd = []string{"sh", "-c",
	`exec psql --no-password -U "$POSTGRES_USER" -Atq -c "SELECT extract(epoch from now()) || ' ' || pg_walfile_name(pg_current_wal_lsn())"`,
}

func switchWAL(ctx context.Context, rt Runtime, containerName string) error {
	rc, err := rt.Exec(ctx, containerName, pgSwitchWALCmd)
	if err != nil {
		return fmt.Errorf("backup: switch wal: %w", err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("backup: switch wal: %w", err)
	}
	return nil
}

var pgSwitchWALCmd = []string{"sh", "-c", `exec psql --no-password -U "$POSTGRES_USER" -Atq -c "SELECT pg_switch_wal()"`}

// archived reports whether walFile is already present in the container's
// own wal-archive mount (postgresWALArchivePath above).
func archived(ctx context.Context, rt Runtime, containerName, walFile string) bool {
	rc, err := rt.Exec(ctx, containerName, []string{"test", "-f", postgresWALArchivePath + "/" + walFile})
	if err != nil {
		return false
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err == nil
}
