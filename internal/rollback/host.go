package rollback

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxBackupOptions = 10

// ListBackups returns the newest local control plane backups with their
// schema versions (-1 when a file cannot be inspected).
func ListBackups(ctx context.Context, dataDir string) ([]BackupOption, error) {
	infos, err := cpbackup.NewManager(nil, dataDir).List()
	if err != nil {
		return nil, fmt.Errorf("list control plane backups: %w", err)
	}
	if len(infos) > maxBackupOptions {
		infos = infos[:maxBackupOptions]
	}
	out := make([]BackupOption, 0, len(infos))
	for _, in := range infos {
		v, err := store.InspectSnapshot(ctx, filepath.Join(dataDir, cpbackup.DirName, in.Name))
		if err != nil {
			v = -1
		}
		out = append(out, BackupOption{Name: in.Name, CreatedAt: in.CreatedAt, SchemaVersion: v})
	}
	return out, nil
}

// DBSchemaVersion reads the schema version of the database file read-only.
// It never migrates, so it is safe against a database newer or older than
// the running binary.
func DBSchemaVersion(ctx context.Context, dbPath string) (int, error) {
	v, err := store.InspectSnapshot(ctx, dbPath)
	if err != nil {
		return -1, fmt.Errorf("read database schema version: %w", err)
	}
	return v, nil
}

var dbSideFiles = []string{"-wal", "-shm"}

// SwapInBackup moves the live database (and its WAL files) aside under a
// timestamped name and copies backupPath into place. It returns the aside
// path for UndoSwap.
func SwapInBackup(dbPath, backupPath string, now time.Time) (string, error) {
	aside := dbPath + ".before-rollback-" + now.UTC().Format("20060102T150405Z")
	if err := os.Rename(dbPath, aside); err != nil {
		return "", fmt.Errorf("move current database aside: %w", err)
	}
	for _, s := range dbSideFiles {
		if err := os.Rename(dbPath+s, aside+s); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("move %s aside: %w", s, err)
		}
	}
	in, err := os.Open(backupPath) //nolint:gosec // name validated against the backup pattern by the caller
	if err != nil {
		_ = UndoSwap(dbPath, aside)
		return "", fmt.Errorf("open backup: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // operator-controlled data dir
	if err != nil {
		_ = UndoSwap(dbPath, aside)
		return "", fmt.Errorf("create database: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = UndoSwap(dbPath, aside)
		return "", fmt.Errorf("copy backup into place: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = UndoSwap(dbPath, aside)
		return "", fmt.Errorf("close database: %w", err)
	}
	return aside, nil
}

// UndoSwap puts the database kept by SwapInBackup back in place.
func UndoSwap(dbPath, aside string) error {
	_ = os.Remove(dbPath)
	for _, s := range dbSideFiles {
		_ = os.Remove(dbPath + s)
	}
	if err := os.Rename(aside, dbPath); err != nil {
		return fmt.Errorf("put previous database back: %w", err)
	}
	for _, s := range dbSideFiles {
		if err := os.Rename(aside+s, dbPath+s); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("put %s back: %w", s, err)
		}
	}
	return nil
}

// WaitHealthy polls base+"/healthz" and base+"/readyz" until both return 2xx
// or timeout passes.
func WaitHealthy(ctx context.Context, base string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last error
	for {
		last = probe(ctx, base+"/healthz")
		if last == nil {
			last = probe(ctx, base+"/readyz")
		}
		if last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("not healthy within %s: %w", timeout, last)
		case <-time.After(time.Second):
		}
	}
}

func probe(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	return nil
}

// WriteAudit inserts one audit_log row straight into the database file,
// without running migrations: the host process may be newer than the schema
// it just rolled back to, and opening the store would migrate it forward.
func WriteAudit(ctx context.Context, dbPath, actor, path string, now time.Time) error {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return err
	}
	sdb, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open database for audit: %w", err)
	}
	defer func() { _ = sdb.Close() }()
	_, err = sdb.ExecContext(ctx, `
		INSERT INTO audit_log (id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind)
		VALUES (?, 'system', 'host-cli', ?, 'root', 'HOST', ?, 200, 'local', ?, 'system')`,
		id, actor, path, store.FormatAuditTime(now))
	if err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}
