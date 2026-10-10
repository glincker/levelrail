package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Major upgrade phases, in order. A failure at or after phaseWipe is rolled
// back from the snapshot taken in phaseSnapshot.
const (
	phasePreflight = "preflight"
	phaseSuspend   = "suspend"
	phaseSnapshot  = "snapshot"
	phaseDump      = "dump"
	phaseWipe      = "wipe"
	phaseUpgrade   = "upgrade"
	phaseRestore   = "restore"
	phaseVerify    = "verify"
	phaseRollback  = "rollback"
	phaseDone      = "done"
)

const (
	postgresDataMount        = "/var/lib/postgresql/data"
	pgDumpCompleteMarker     = "PostgreSQL database dump complete"
	defaultUpgradeWait       = 5 * time.Minute
	defaultUpgradePoll       = time.Second
	snapshotSrcMount         = "/src"
	snapshotDstMount         = "/dst"
	upgradeSourceNamePattern = "db-%s-upgrade-src-%s"
)

// pgTableCountsSQL prints one "table=rows" list for every public table, used
// to compare the old cluster with the restored one.
const pgTableCountsSQL = `SELECT coalesce(string_agg(table_name || '=' || (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %I.%I', table_schema, table_name), false, true, '')))[1]::text, ',' ORDER BY table_name), '') FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';`

// MajorUpgradeStore is the store surface MajorUpgradeRunner needs.
type MajorUpgradeStore interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	SaveDesiredDatabase(ctx context.Context, d store.DesiredDatabase) error
	UpdateDatabaseSuspended(ctx context.Context, name string, suspended bool) error
	StartMajorUpgrade(ctx context.Context, u store.MajorUpgrade) error
	UpdateMajorUpgradePhase(ctx context.Context, id, phase, snapshotVolume string) error
	FinishMajorUpgrade(ctx context.Context, id, status, errMsg, finishedAt string) error
	GetMajorUpgrade(ctx context.Context, id string) (store.MajorUpgrade, error)
	ListRunningMajorUpgrades(ctx context.Context) ([]store.MajorUpgrade, error)
	ClearMajorUpgradeSnapshot(ctx context.Context, id string) error
}

// VolumeRuntime is docker.Runtime plus volume removal.
type VolumeRuntime interface {
	docker.Runtime
	RemoveVolume(ctx context.Context, name string) error
}

// MajorUpgradeRunner upgrades a managed Postgres database across a major
// version by dump and restore. The old data volume is never rewritten in
// place: it is copied to a snapshot volume first, the dump is taken from the
// snapshot, and any failure after the live volume is wiped restores the
// snapshot and the old version.
type MajorUpgradeRunner struct {
	Store    MajorUpgradeStore
	Runtime  VolumeRuntime
	Dumper   Dumper
	Restorer Restorer
	Nudge    func()
	Now      func() time.Time
	WorkDir  string
	Logger   *slog.Logger
	// Wait bounds each wait for a container to stop, start or accept
	// connections; Poll is the interval between checks.
	Wait time.Duration
	Poll time.Duration
}

type upgradeRun struct {
	id, name, container, dataVolume string
	from, to                        string
	snapshot                        string
	suspended, wiped                bool
}

// RunMajorUpgrade runs the upgrade of databaseName to toVersion and records
// every phase under historyID. It returns the failure, if any, after the
// rollback has run.
func (r *MajorUpgradeRunner) RunMajorUpgrade(ctx context.Context, historyID, databaseName, toVersion string) error {
	desired, err := r.Store.GetDesiredDatabase(ctx, databaseName)
	if err != nil {
		return fmt.Errorf("load database %q: %w", databaseName, err)
	}
	run := &upgradeRun{
		id: historyID, name: databaseName, container: "db-" + databaseName, dataVolume: "db-" + databaseName + "-data",
		from: desired.Version, to: toVersion,
	}
	if err := r.Store.StartMajorUpgrade(ctx, store.MajorUpgrade{
		ID: historyID, DatabaseName: databaseName, FromVersion: desired.Version, ToVersion: toVersion,
		StartedAt: r.now().UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("start major upgrade history %q: %w", historyID, err)
	}

	runErr := r.upgrade(ctx, run, desired)
	if runErr == nil {
		return r.finish(historyID, store.BackupStatusSucceeded, "")
	}

	bg := context.WithoutCancel(ctx)
	if !run.wiped {
		r.abortBeforeWipe(bg, run)
		if ferr := r.finish(historyID, store.BackupStatusFailed, runErr.Error()); ferr != nil {
			return errors.Join(runErr, ferr)
		}
		return runErr
	}
	if rbErr := r.rollbackToSnapshot(bg, run); rbErr != nil {
		msg := fmt.Sprintf("%v; ROLLBACK FAILED: the pre-upgrade data is intact in volume %q: %v", runErr, run.snapshot, rbErr)
		_ = r.finish(historyID, store.BackupStatusFailed, msg)
		return errors.New(msg)
	}
	msg := fmt.Sprintf("%v; rolled back to version %s", runErr, run.from)
	if ferr := r.finish(historyID, store.MajorUpgradeStatusRolledBack, msg); ferr != nil {
		return errors.Join(errors.New(msg), ferr)
	}
	return errors.New(msg)
}

func (r *MajorUpgradeRunner) upgrade(ctx context.Context, run *upgradeRun, desired *store.DesiredDatabase) error {
	if err := r.phase(ctx, run, phasePreflight); err != nil {
		return err
	}
	if err := checkDiskSpace(resolveWorkDir(r.WorkDir)); err != nil {
		return err
	}
	if err := r.checkTargetImage(ctx, run); err != nil {
		return err
	}

	if err := r.phase(ctx, run, phaseSuspend); err != nil {
		return err
	}
	if err := r.Store.UpdateDatabaseSuspended(ctx, run.name, true); err != nil {
		return fmt.Errorf("suspend database %q: %w", run.name, err)
	}
	run.suspended = true
	r.nudge()
	if err := r.waitContainerGone(ctx, run.container); err != nil {
		return err
	}

	if err := r.phase(ctx, run, phaseSnapshot); err != nil {
		return err
	}
	run.snapshot = fmt.Sprintf("db-%s-data-pre%s-%s", run.name, majorLabel(run.from), shortID(run.id))
	if err := r.Runtime.EnsureVolume(ctx, run.snapshot); err != nil {
		return fmt.Errorf("create snapshot volume %q: %w", run.snapshot, err)
	}
	if err := r.Store.UpdateMajorUpgradePhase(ctx, run.id, phaseSnapshot, run.snapshot); err != nil {
		return fmt.Errorf("record snapshot volume: %w", err)
	}
	if err := r.copyVolume(ctx, run.dataVolume, run.snapshot); err != nil {
		return fmt.Errorf("snapshot data volume: %w", err)
	}

	if err := r.phase(ctx, run, phaseDump); err != nil {
		return err
	}
	dumpPath, wantCounts, err := r.dumpFromSnapshot(ctx, run)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(dumpPath) }()

	if err := r.phase(ctx, run, phaseWipe); err != nil {
		return err
	}
	run.wiped = true
	if err := r.wipeVolume(ctx, run.dataVolume); err != nil {
		return err
	}

	if err := r.phase(ctx, run, phaseUpgrade); err != nil {
		return err
	}
	next := *desired
	next.Version = run.to
	if err := r.Store.SaveDesiredDatabase(ctx, next); err != nil {
		return fmt.Errorf("set database version %q: %w", run.to, err)
	}
	if err := r.Store.UpdateDatabaseSuspended(ctx, run.name, false); err != nil {
		return fmt.Errorf("unsuspend database %q: %w", run.name, err)
	}
	run.suspended = false
	r.nudge()
	if err := r.waitContainerRunning(ctx, run.container, run.to); err != nil {
		return err
	}

	if err := r.phase(ctx, run, phaseRestore); err != nil {
		return err
	}
	f, err := os.Open(dumpPath) //nolint:gosec // path created by this runner in the work dir
	if err != nil {
		return fmt.Errorf("open dump: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := r.Restorer.Restore(ctx, store.EnginePostgres, run.container, f); err != nil {
		return fmt.Errorf("restore into version %s: %w", run.to, err)
	}

	if err := r.phase(ctx, run, phaseVerify); err != nil {
		return err
	}
	return r.verify(ctx, run, wantCounts)
}

// checkTargetImage creates (and removes) a container from the target image so
// a version that does not exist fails before the database is taken offline.
func (r *MajorUpgradeRunner) checkTargetImage(ctx context.Context, run *upgradeRun) error {
	id, err := r.Runtime.Create(ctx, docker.ContainerSpec{
		Name: fmt.Sprintf("db-%s-upgrade-img-%s", run.name, shortID(run.id)), Image: database.ImageRef(store.EnginePostgres, run.to),
		Command: []string{"true"},
	})
	if err != nil {
		return fmt.Errorf("target image %s is not available: %w", database.ImageRef(store.EnginePostgres, run.to), err)
	}
	if err := r.Runtime.Remove(ctx, id, true); err != nil {
		return fmt.Errorf("remove image check container: %w", err)
	}
	return nil
}

func (r *MajorUpgradeRunner) phase(ctx context.Context, run *upgradeRun, name string) error {
	if err := r.Store.UpdateMajorUpgradePhase(ctx, run.id, name, ""); err != nil {
		return fmt.Errorf("record phase %q: %w", name, err)
	}
	return nil
}

func (r *MajorUpgradeRunner) abortBeforeWipe(ctx context.Context, run *upgradeRun) {
	if run.suspended {
		if err := r.Store.UpdateDatabaseSuspended(ctx, run.name, false); err != nil {
			r.log().Error("backup: major upgrade: unsuspend after abort failed", slog.String("database", run.name), slog.String("error", err.Error()))
		}
		r.nudge()
	}
	if run.snapshot != "" {
		if err := r.Runtime.RemoveVolume(ctx, run.snapshot); err != nil {
			r.log().Warn("backup: major upgrade: snapshot volume left behind", slog.String("volume", run.snapshot), slog.String("error", err.Error()))
			return
		}
		_ = r.Store.ClearMajorUpgradeSnapshot(ctx, run.id)
	}
}

// rollbackToSnapshot puts the snapshot back as the live data volume and the
// old version back in desired state. Safe to repeat.
func (r *MajorUpgradeRunner) rollbackToSnapshot(ctx context.Context, run *upgradeRun) error {
	_ = r.Store.UpdateMajorUpgradePhase(ctx, run.id, phaseRollback, "")
	if err := r.Store.UpdateDatabaseSuspended(ctx, run.name, true); err != nil {
		return fmt.Errorf("suspend database %q: %w", run.name, err)
	}
	r.nudge()
	if err := r.waitContainerGone(ctx, run.container); err != nil {
		return err
	}
	if err := r.wipeVolume(ctx, run.dataVolume); err != nil {
		return err
	}
	if err := r.copyVolume(ctx, run.snapshot, run.dataVolume); err != nil {
		return fmt.Errorf("copy snapshot back: %w", err)
	}
	desired, err := r.Store.GetDesiredDatabase(ctx, run.name)
	if err != nil {
		return fmt.Errorf("load database %q: %w", run.name, err)
	}
	prev := *desired
	prev.Version = run.from
	if err := r.Store.SaveDesiredDatabase(ctx, prev); err != nil {
		return fmt.Errorf("restore database version %q: %w", run.from, err)
	}
	if err := r.Store.UpdateDatabaseSuspended(ctx, run.name, false); err != nil {
		return fmt.Errorf("unsuspend database %q: %w", run.name, err)
	}
	r.nudge()
	return r.waitContainerRunning(ctx, run.container, run.from)
}

// Rollback undoes a finished upgrade by restoring its snapshot. Writes made
// since the upgrade are lost.
func (r *MajorUpgradeRunner) Rollback(ctx context.Context, upgradeID string) error {
	u, err := r.Store.GetMajorUpgrade(ctx, upgradeID)
	if err != nil {
		return fmt.Errorf("load upgrade %q: %w", upgradeID, err)
	}
	if u.Status == store.BackupStatusRunning {
		return fmt.Errorf("upgrade %q is still running", upgradeID)
	}
	if u.SnapshotVolume == "" {
		return fmt.Errorf("upgrade %q has no rollback snapshot (it was discarded or never taken)", upgradeID)
	}
	run := &upgradeRun{
		id: u.ID, name: u.DatabaseName, container: "db-" + u.DatabaseName, dataVolume: "db-" + u.DatabaseName + "-data",
		from: u.FromVersion, to: u.ToVersion, snapshot: u.SnapshotVolume,
	}
	if err := r.rollbackToSnapshot(ctx, run); err != nil {
		_ = r.finish(u.ID, store.BackupStatusFailed, "ROLLBACK FAILED: "+err.Error())
		return err
	}
	return r.finish(u.ID, store.MajorUpgradeStatusRolledBack, "rolled back to version "+u.FromVersion+" by an operator")
}

// DiscardSnapshot deletes an upgrade's rollback snapshot volume.
func (r *MajorUpgradeRunner) DiscardSnapshot(ctx context.Context, upgradeID string) error {
	u, err := r.Store.GetMajorUpgrade(ctx, upgradeID)
	if err != nil {
		return fmt.Errorf("load upgrade %q: %w", upgradeID, err)
	}
	if u.Status == store.BackupStatusRunning {
		return fmt.Errorf("upgrade %q is still running", upgradeID)
	}
	if u.SnapshotVolume == "" {
		return nil
	}
	if err := r.Runtime.RemoveVolume(ctx, u.SnapshotVolume); err != nil {
		return fmt.Errorf("remove snapshot volume %q: %w", u.SnapshotVolume, err)
	}
	return r.Store.ClearMajorUpgradeSnapshot(ctx, upgradeID)
}

// RecoverInterrupted settles upgrades left running by a control plane restart:
// before the wipe it unsuspends the database, after it it rolls back.
func (r *MajorUpgradeRunner) RecoverInterrupted(ctx context.Context) {
	running, err := r.Store.ListRunningMajorUpgrades(ctx)
	if err != nil {
		r.log().Error("backup: major upgrade recovery: list failed", slog.String("error", err.Error()))
		return
	}
	for _, u := range running {
		run := &upgradeRun{
			id: u.ID, name: u.DatabaseName, container: "db-" + u.DatabaseName, dataVolume: "db-" + u.DatabaseName + "-data",
			from: u.FromVersion, to: u.ToVersion, snapshot: u.SnapshotVolume, suspended: true,
		}
		interrupted := fmt.Sprintf("interrupted by a control plane restart during phase %q", u.Phase)
		switch u.Phase {
		case phaseWipe, phaseUpgrade, phaseRestore, phaseVerify, phaseRollback:
			if run.snapshot == "" {
				_ = r.finish(u.ID, store.BackupStatusFailed, interrupted+" and no snapshot exists: restore from a backup")
				continue
			}
			if err := r.rollbackToSnapshot(ctx, run); err != nil {
				_ = r.finish(u.ID, store.BackupStatusFailed, fmt.Sprintf("%s; ROLLBACK FAILED, the pre-upgrade data is intact in volume %q: %v", interrupted, run.snapshot, err))
				continue
			}
			_ = r.finish(u.ID, store.MajorUpgradeStatusRolledBack, interrupted+"; rolled back to version "+u.FromVersion)
		default:
			r.abortBeforeWipe(ctx, run)
			_ = r.finish(u.ID, store.BackupStatusFailed, interrupted)
		}
	}
}

func (r *MajorUpgradeRunner) dumpFromSnapshot(ctx context.Context, run *upgradeRun) (path, counts string, err error) {
	srcName := fmt.Sprintf(upgradeSourceNamePattern, run.name, shortID(run.id))
	env := map[string]string{"POSTGRES_USER": run.name, "POSTGRES_PASSWORD": "unused-existing-data-dir"}
	if database.PostgresNeedsPGDATA(run.from) {
		env["PGDATA"] = postgresDataMount
	}
	id, err := r.Runtime.Create(ctx, docker.ContainerSpec{
		Name: srcName, Image: database.ImageRef(store.EnginePostgres, run.from), Env: env,
		Volumes: []docker.VolumeMount{{Name: run.snapshot, ContainerPath: postgresDataMount}},
	})
	if err != nil {
		return "", "", fmt.Errorf("create source container: %w", err)
	}
	defer func() {
		_ = r.Runtime.Stop(context.WithoutCancel(ctx), id, 30*time.Second)
		_ = r.Runtime.Remove(context.WithoutCancel(ctx), id, true)
	}()
	if err := r.Runtime.Start(ctx, id); err != nil {
		return "", "", fmt.Errorf("start source container: %w", err)
	}
	if err := r.waitSQL(ctx, srcName); err != nil {
		return "", "", fmt.Errorf("old cluster on the snapshot did not start: %w", err)
	}
	if counts, err = r.tableCounts(ctx, srcName); err != nil {
		return "", "", fmt.Errorf("count rows in the old cluster: %w", err)
	}

	dir := resolveWorkDir(r.WorkDir)
	f, err := os.CreateTemp(dir, "major-upgrade-*.sql")
	if err != nil {
		return "", "", fmt.Errorf("create dump file: %w", err)
	}
	path = f.Name()
	defer func() {
		_ = f.Close()
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	rc, err := r.Dumper.Dump(ctx, store.EnginePostgres, srcName)
	if err != nil {
		return "", "", fmt.Errorf("dump old cluster: %w", err)
	}
	defer func() { _ = rc.Close() }()
	if _, err = io.Copy(f, rc); err != nil {
		return "", "", fmt.Errorf("dump old cluster: %w", err)
	}
	if err = dumpIsComplete(f); err != nil {
		return "", "", err
	}
	return path, counts, nil
}

func dumpIsComplete(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat dump: %w", err)
	}
	tail := int64(512)
	if info.Size() < tail {
		tail = info.Size()
	}
	buf := make([]byte, tail)
	if _, err := f.ReadAt(buf, info.Size()-tail); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read dump tail: %w", err)
	}
	if !bytes.Contains(buf, []byte(pgDumpCompleteMarker)) {
		return errors.New("dump of the old cluster is truncated: missing the pg_dump completion marker")
	}
	return nil
}

func (r *MajorUpgradeRunner) verify(ctx context.Context, run *upgradeRun, wantCounts string) error {
	out, err := r.execSQL(ctx, run.container, "SHOW server_version_num;")
	if err != nil {
		return fmt.Errorf("read server version: %w", err)
	}
	num, perr := strconv.Atoi(strings.TrimSpace(out))
	wantMajor, _ := strconv.Atoi(majorLabel(run.to))
	if perr != nil || num/10000 != wantMajor {
		return fmt.Errorf("restored server reports version %q, expected major %d", strings.TrimSpace(out), wantMajor)
	}
	got, err := r.tableCounts(ctx, run.container)
	if err != nil {
		return fmt.Errorf("count rows after restore: %w", err)
	}
	if got != wantCounts {
		return fmt.Errorf("row counts differ after restore: before %q, after %q", wantCounts, got)
	}
	return nil
}

func (r *MajorUpgradeRunner) tableCounts(ctx context.Context, container string) (string, error) {
	out, err := r.execSQL(ctx, container, pgTableCountsSQL)
	return strings.TrimSpace(out), err
}

func (r *MajorUpgradeRunner) execSQL(ctx context.Context, container, sql string) (string, error) {
	rc, err := r.Runtime.ExecWithInput(ctx, container, []string{"sh", "-c", `exec psql --no-password -v ON_ERROR_STOP=1 -Atq -U "$POSTGRES_USER" "$POSTGRES_USER"`}, strings.NewReader(sql))
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	return string(b), err
}

func (r *MajorUpgradeRunner) waitSQL(ctx context.Context, container string) error {
	deadline := time.Now().Add(r.wait())
	for {
		_, err := r.execSQL(ctx, container, "SELECT 1;")
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return err
		}
		if err := sleepCtx(ctx, r.poll()); err != nil {
			return err
		}
	}
}

// copyVolume copies every file of src into dst, keeping modes and owners.
func (r *MajorUpgradeRunner) copyVolume(ctx context.Context, src, dst string) error {
	suffix, err := randomHelperSuffix()
	if err != nil {
		return err
	}
	id, err := r.Runtime.Create(ctx, docker.ContainerSpec{
		Name: "volcopy-" + suffix, Image: volumeHelperImage, Command: []string{"sleep", "86400"},
		Volumes: []docker.VolumeMount{{Name: src, ContainerPath: snapshotSrcMount, ReadOnly: true}, {Name: dst, ContainerPath: snapshotDstMount}},
	})
	if err != nil {
		return fmt.Errorf("create copy helper: %w", err)
	}
	defer func() { _ = r.Runtime.Remove(context.WithoutCancel(ctx), id, true) }()
	if err := r.Runtime.Start(ctx, id); err != nil {
		return fmt.Errorf("start copy helper: %w", err)
	}
	_, err = execOutput(ctx, r.Runtime, id, []string{"sh", "-c", "cp -a " + snapshotSrcMount + "/. " + snapshotDstMount + "/"})
	if err != nil {
		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}
	return nil
}

func (r *MajorUpgradeRunner) wipeVolume(ctx context.Context, name string) error {
	id, err := createVolumeHelper(ctx, r.Runtime, name, "pgwipe", false)
	if err != nil {
		return fmt.Errorf("open volume %q: %w", name, err)
	}
	defer func() { _ = r.Runtime.Remove(context.WithoutCancel(ctx), id, true) }()
	cmd := []string{"sh", "-c", "cd " + volumeMountPath + " && rm -rf -- ..?* .[!.]* *"}
	if _, err := execOutput(ctx, r.Runtime, id, cmd); err != nil {
		return fmt.Errorf("wipe volume %q: %w", name, err)
	}
	return nil
}

func (r *MajorUpgradeRunner) waitContainerGone(ctx context.Context, name string) error {
	deadline := time.Now().Add(r.wait())
	for {
		state, err := r.Runtime.InspectByName(ctx, name)
		if err != nil {
			return fmt.Errorf("inspect %q: %w", name, err)
		}
		if state == nil || !state.Running {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("container %q was not stopped within %s", name, r.wait())
		}
		if err := sleepCtx(ctx, r.poll()); err != nil {
			return err
		}
	}
}

// waitContainerRunning waits for the reconciler to bring the container up on
// the expected version's image.
func (r *MajorUpgradeRunner) waitContainerRunning(ctx context.Context, name, version string) error {
	deadline := time.Now().Add(r.wait())
	for {
		state, err := r.Runtime.InspectByName(ctx, name)
		if err == nil && state != nil && state.Running && strings.HasSuffix(state.Image, ":"+database.ImageTag(store.EnginePostgres, version)) {
			return r.waitSQL(ctx, name)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("container %q did not come up on version %s within %s", name, version, r.wait())
		}
		if err := sleepCtx(ctx, r.poll()); err != nil {
			return err
		}
	}
}

func (r *MajorUpgradeRunner) finish(id, status, msg string) error {
	return r.Store.FinishMajorUpgrade(context.Background(), id, status, msg, r.now().UTC().Format(time.RFC3339))
}

func (r *MajorUpgradeRunner) nudge() {
	if r.Nudge != nil {
		r.Nudge()
	}
}

func (r *MajorUpgradeRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *MajorUpgradeRunner) wait() time.Duration {
	if r.Wait > 0 {
		return r.Wait
	}
	return defaultUpgradeWait
}

func (r *MajorUpgradeRunner) poll() time.Duration {
	if r.Poll > 0 {
		return r.Poll
	}
	return defaultUpgradePoll
}

func (r *MajorUpgradeRunner) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func majorLabel(version string) string {
	end := 0
	for end < len(version) && version[end] >= '0' && version[end] <= '9' {
		end++
	}
	return version[:end]
}

// shortID keeps the alphanumeric characters of an id for use in a volume name.
func shortID(id string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, id))
}
