package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MySQLPITRStore is the store surface MySQLPITRRunner needs.
type MySQLPITRStore interface {
	TargetGetter
	GetBaseBackupHistory(ctx context.Context, id string) (store.BaseBackupHistory, error)
	GetOldestSucceededBaseBackup(ctx context.Context, databaseName string) (store.BaseBackupHistory, bool, error)
	StartPITRRestoreHistory(ctx context.Context, h store.PITRRestoreHistory) error
	FinishPITRRestoreHistory(ctx context.Context, id, status, errMsg, finishedAt string) error
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
}

const (
	defaultMySQLPITRReadyTimeout  = 10 * time.Minute
	defaultMySQLPITRSettleTimeout = 60 * time.Second
	defaultMySQLPITRPollInterval  = 2 * time.Second
)

// MySQLPITRRunner restores a MySQL or MariaDB database to a point in time by
// loading a base backup dump into a brand-new database and replaying the
// shipped binlog on top of it. The source database is never written to.
type MySQLPITRRunner struct {
	Store      MySQLPITRStore
	Secrets    SecretsResolver
	Downloader Downloader
	Lister     Lister
	Restorer   Restorer
	// Shipper, when set, ships the source's closed binlogs first so the replay
	// sees everything up to the requested target.
	Shipper WALShipping
	Runtime docker.Runtime
	// ToolsImage overrides the binlog reader image; see EnvMySQLToolsImage.
	ToolsImage string
	Now        func() time.Time
	// ReadyTimeout, SettleTimeout and PollInterval override the defaults when non-zero.
	ReadyTimeout  time.Duration
	SettleTimeout time.Duration
	PollInterval  time.Duration
}

func (r *MySQLPITRRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func pick(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

// Window computes the currently recoverable range: it starts once the oldest
// succeeded base backup has finished (a dump is only consistent as of its snapshot,
// which lies between its start and finish) and ends at the last rotated binlog.
func (r *MySQLPITRRunner) Window(ctx context.Context, databaseName, containerName string) (PITRWindow, error) {
	oldest, ok, err := r.Store.GetOldestSucceededBaseBackup(ctx, databaseName)
	if err != nil {
		return PITRWindow{}, fmt.Errorf("get oldest succeeded base backup for %q: %w", databaseName, err)
	}
	if !ok {
		return PITRWindow{}, ErrNoBaseBackup
	}
	start, err := baseBackupSettledAt(oldest)
	if err != nil {
		return PITRWindow{}, err
	}
	end, err := MySQLWindowEnd(ctx, r.Runtime, containerName)
	if err != nil {
		return PITRWindow{}, err
	}
	return PITRWindow{HasBaseBackup: true, Start: start, End: end}, nil
}

func baseBackupSettledAt(h store.BaseBackupHistory) (time.Time, error) {
	raw := h.FinishedAt
	if raw == "" {
		raw = h.StartedAt
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse base backup %q time %q: %w", h.ID, raw, err)
	}
	return t, nil
}

// WindowBounds is Window destructured for the API package boundary.
func (r *MySQLPITRRunner) WindowBounds(ctx context.Context, databaseName, containerName string) (hasBaseBackup bool, start, end time.Time, err error) {
	win, err := r.Window(ctx, databaseName, containerName)
	if err != nil {
		return false, time.Time{}, time.Time{}, err
	}
	return win.HasBaseBackup, win.Start, win.End, nil
}

// ValidateTarget checks target falls inside databaseName's current window.
func (r *MySQLPITRRunner) ValidateTarget(ctx context.Context, databaseName, containerName string, target time.Time) error {
	win, err := r.Window(ctx, databaseName, containerName)
	if err != nil {
		return err
	}
	return ValidatePITRTarget(win, target)
}

// RunPITRCloneRestore restores baseBackupHistoryID plus the binlog up to target
// into newDatabaseName, recording the attempt against the source database.
// Expected to run in a goroutine the caller has already detached.
func (r *MySQLPITRRunner) RunPITRCloneRestore(ctx context.Context, historyID, sourceName, newName, baseBackupHistoryID, engine, sourceContainer, newContainer, newController string, target time.Time) error {
	if err := r.Store.StartPITRRestoreHistory(ctx, store.PITRRestoreHistory{
		ID:                  historyID,
		DatabaseName:        sourceName,
		BaseBackupHistoryID: baseBackupHistoryID,
		TargetTimestamp:     target.UTC().Format(time.RFC3339),
		StartedAt:           r.now().UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start pitr restore history %q: %w", historyID, err)
	}

	runErr := r.run(ctx, sourceName, newName, baseBackupHistoryID, engine, sourceContainer, newContainer, newController, target)

	status, errMsg := store.BackupStatusSucceeded, ""
	if runErr != nil {
		status, errMsg = store.BackupStatusFailed, runErr.Error()
	}
	if err := r.Store.FinishPITRRestoreHistory(ctx, historyID, status, errMsg, r.now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("backup: finish pitr restore history %q: %w", historyID, err)
	}
	return runErr
}

func (r *MySQLPITRRunner) run(ctx context.Context, sourceName, newName, baseBackupHistoryID, engine, sourceContainer, newContainer, newController string, target time.Time) error {
	bh, err := r.Store.GetBaseBackupHistory(ctx, baseBackupHistoryID)
	if err != nil {
		return fmt.Errorf("get base backup history %q: %w", baseBackupHistoryID, err)
	}
	if bh.Status != store.BackupStatusSucceeded {
		return fmt.Errorf("base backup %q has status %q, not %q: refusing to restore from an attempt that did not succeed", baseBackupHistoryID, bh.Status, store.BackupStatusSucceeded)
	}
	settled, err := baseBackupSettledAt(bh)
	if err != nil {
		return err
	}
	if target.Before(settled) {
		return fmt.Errorf("target time %s is before base backup %q finished at %s", target.Format(time.RFC3339), bh.ID, settled.Format(time.RFC3339))
	}
	m := binlogCoordRe.FindStringSubmatch(bh.LSN)
	if m == nil {
		return fmt.Errorf("base backup %q has no binlog position (%q): it cannot seed a point-in-time restore", bh.ID, bh.LSN)
	}
	startPos, _ := strconv.ParseInt(m[2], 10, 64)

	dest, err := resolveTargetDestination(ctx, r.Store, r.Secrets, bh.TargetID)
	if err != nil {
		return err
	}
	if err := r.shipSource(ctx, dest, sourceName, sourceContainer); err != nil {
		return err
	}

	files, err := r.binlogsFrom(ctx, dest, sourceName, m[1])
	if err != nil {
		return err
	}
	if len(files) == 0 || files[0] != m[1] {
		return fmt.Errorf("binlog %q that the base backup starts from is not in the backup target: the recoverable history is incomplete", m[1])
	}

	if err := r.waitUntilReady(ctx, newController); err != nil {
		return fmt.Errorf("wait for database %q to become ready: %w", newName, err)
	}
	if err := r.loadDump(ctx, dest, bh, engine, newContainer); err != nil {
		return err
	}
	return r.replay(ctx, dest, sourceName, newName, newContainer, files, startPos, target)
}

func (r *MySQLPITRRunner) shipSource(ctx context.Context, dest Destination, sourceName, sourceContainer string) error {
	if r.Shipper == nil {
		return nil
	}
	state, err := r.Runtime.InspectByName(ctx, sourceContainer)
	if err != nil || state == nil || !state.Running {
		return nil
	}
	if _, err := r.Shipper.Ship(ctx, dest, sourceName, sourceContainer); err != nil {
		return fmt.Errorf("ship binlogs of %q before restore: %w", sourceName, err)
	}
	return nil
}

func (r *MySQLPITRRunner) binlogsFrom(ctx context.Context, dest Destination, sourceName, firstFile string) ([]string, error) {
	objs, err := r.Lister.List(ctx, dest, BinlogPrefix(sourceName))
	if err != nil {
		return nil, fmt.Errorf("list shipped binlogs for %q: %w", sourceName, err)
	}
	var files []string
	for _, o := range objs {
		name := strings.TrimPrefix(o.Key, BinlogPrefix(sourceName))
		if binlogNameRe.MatchString(name) && name >= firstFile {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	return files, nil
}

func (r *MySQLPITRRunner) loadDump(ctx context.Context, dest Destination, bh store.BaseBackupHistory, engine, newContainer string) error {
	deadline := time.Now().Add(pick(r.SettleTimeout, defaultMySQLPITRSettleTimeout))
	for {
		err := r.loadDumpOnce(ctx, dest, bh, engine, newContainer)
		if err == nil || !time.Now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(pick(r.PollInterval, defaultMySQLPITRPollInterval)):
		}
	}
}

func (r *MySQLPITRRunner) loadDumpOnce(ctx context.Context, dest Destination, bh store.BaseBackupHistory, engine, newContainer string) error {
	dump, err := r.Downloader.Download(ctx, dest, bh.ObjectKey)
	if err != nil {
		return fmt.Errorf("download base backup %q object %q: %w", bh.ID, bh.ObjectKey, err)
	}
	defer func() { _ = dump.Close() }()
	if err := r.Restorer.Restore(ctx, engine, newContainer, dump); err != nil {
		return fmt.Errorf("load base backup %q: %w", bh.ID, err)
	}
	return nil
}

// replay stages the shipped binlog files where a binlog reader is, converts them
// to SQL and loads that into the new database. The MariaDB image ships its reader;
// the MySQL image does not, so a short-lived tools container supplies one.
func (r *MySQLPITRRunner) replay(ctx context.Context, dest Destination, sourceName, newName, newContainer string, files []string, startPos int64, target time.Time) error {
	tool := newContainer
	if err := r.exec(ctx, newContainer, binlogToolCmd, nil); err != nil {
		id, cleanup, herr := r.startToolsContainer(ctx)
		if herr != nil {
			return herr
		}
		defer cleanup()
		tool = id
	}

	if err := r.exec(ctx, tool, []string{"mkdir", "-p", "/tmp/pitr-binlog"}, nil); err != nil {
		return fmt.Errorf("stage binlogs: %w", err)
	}
	for _, name := range files {
		body, err := r.Downloader.Download(ctx, dest, BinlogPrefix(sourceName)+name)
		if err != nil {
			return fmt.Errorf("download binlog %q: %w", name, err)
		}
		err = r.exec(ctx, tool, []string{"sh", "-c", "cat > /tmp/pitr-binlog/" + name}, body)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("stage binlog %q: %w", name, err)
		}
	}

	sql, err := r.Runtime.Exec(ctx, tool, binlogSQLCmd(files, startPos, target, sourceName, newName))
	if err != nil {
		return fmt.Errorf("read binlog: %w", err)
	}
	defer func() { _ = sql.Close() }()
	capture := &errCapture{r: sql}
	if err := r.exec(ctx, newContainer, mysqlLoadCmd, capture); err != nil {
		return fmt.Errorf("replay binlog to %s: %w", target.UTC().Format(time.RFC3339), err)
	}
	if capture.err != nil && !errors.Is(capture.err, io.EOF) {
		return fmt.Errorf("read binlog: %w", capture.err)
	}
	return nil
}

// errCapture remembers the read error of a stream consumed as stdin, where the
// exec layer may not surface it.
type errCapture struct {
	r   io.Reader
	err error
}

func (e *errCapture) Read(b []byte) (int, error) {
	n, err := e.r.Read(b)
	if err != nil {
		e.err = err
	}
	return n, err
}

func (r *MySQLPITRRunner) startToolsContainer(ctx context.Context) (string, func(), error) {
	suffix, err := randomHelperSuffix()
	if err != nil {
		return "", nil, fmt.Errorf("generate tools container name: %w", err)
	}
	id, err := r.Runtime.Create(ctx, docker.ContainerSpec{
		Name:       "pitrtools-" + suffix,
		Image:      r.toolsImage(),
		Entrypoint: []string{"sleep"},
		Command:    []string{"3600"},
	})
	if err != nil {
		return "", nil, fmt.Errorf("create binlog tools container from %q: %w", r.toolsImage(), err)
	}
	if err := r.Runtime.Start(ctx, id); err != nil {
		_ = r.Runtime.Remove(context.Background(), id, true)
		return "", nil, fmt.Errorf("start binlog tools container: %w", err)
	}
	return id, func() { _ = r.Runtime.Remove(context.Background(), id, true) }, nil
}

// EnvMySQLToolsImage names the image whose mysqlbinlog reads a MySQL point-in-time
// restore's binlog; the official MySQL image does not ship one.
const EnvMySQLToolsImage = "APP_PITR_MYSQL_TOOLS_IMAGE"

const defaultMySQLToolsImage = "percona/percona-server:8.4"

func (r *MySQLPITRRunner) toolsImage() string {
	if r.ToolsImage != "" {
		return r.ToolsImage
	}
	if v := os.Getenv(EnvMySQLToolsImage); v != "" {
		return v
	}
	return defaultMySQLToolsImage
}

func (r *MySQLPITRRunner) exec(ctx context.Context, container string, cmd []string, stdin io.Reader) error {
	var rc io.ReadCloser
	var err error
	if stdin != nil {
		rc, err = r.Runtime.ExecWithInput(ctx, container, cmd, stdin)
	} else {
		rc, err = r.Runtime.Exec(ctx, container, cmd)
	}
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

func (r *MySQLPITRRunner) waitUntilReady(ctx context.Context, controllerName string) error {
	timeout := pick(r.ReadyTimeout, defaultMySQLPITRReadyTimeout)
	deadline := time.Now().Add(timeout)
	for {
		conditions, err := r.Store.GetConditions(ctx, controllerName)
		if err != nil {
			return fmt.Errorf("get conditions for %q: %w", controllerName, err)
		}
		for _, c := range conditions {
			if c.Type == reconcile.ConditionTypeReady && c.Status == reconcile.ConditionTrue {
				return nil
			}
		}
		if !time.Now().Before(deadline) {
			return errors.New("not ready within " + timeout.String())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pick(r.PollInterval, defaultMySQLPITRPollInterval)):
		}
	}
}
