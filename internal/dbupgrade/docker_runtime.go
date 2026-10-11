package dbupgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// BackupRunner takes a logical backup; *backup.Runner satisfies it.
type BackupRunner interface {
	RunBackup(ctx context.Context, historyID, databaseName, engine, containerName, targetID string) error
}

// VerifyRunner checks a finished backup.
type VerifyRunner interface {
	VerifyBackup(ctx context.Context, verificationID, backupHistoryID, engine, checkedBy string) error
}

// RestoreRunner restores a logical backup into a running database.
type RestoreRunner interface {
	RunRestore(ctx context.Context, historyID, databaseName, backupHistoryID, engine, containerName string) error
}

// DockerStore is the store surface DockerRuntime reads and writes.
type DockerStore interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	SaveDesiredDatabase(ctx context.Context, d store.DesiredDatabase) error
	UpdateDatabaseSuspended(ctx context.Context, name string, suspended bool) error
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
	ListBackupVerifications(ctx context.Context, backupHistoryID string, limit int) ([]store.BackupVerification, error)
	ListRunningMajorUpgrades(ctx context.Context) ([]store.MajorUpgrade, error)
	ListRestoreHistory(ctx context.Context, databaseName string) ([]store.RestoreHistory, error)
}

// VolumeRuntime is docker.Runtime plus volume management.
type VolumeRuntime interface {
	docker.Runtime
	EnsureVolume(ctx context.Context, name string) error
	RemoveVolume(ctx context.Context, name string) error
}

// DockerRuntime implements Runtime against the local Docker daemon and the
// existing backup, verification and restore runners.
type DockerRuntime struct {
	Store    DockerStore
	Docker   VolumeRuntime
	Backups  BackupRunner
	Verifier VerifyRunner
	Restorer RestoreRunner
	// CopyVolume and WipeVolume are internal/backup's volume helpers.
	CopyVolume func(ctx context.Context, rt docker.Runtime, src, dst string) error
	WipeVolume func(ctx context.Context, rt docker.Runtime, name string) error
	Nudge      func()
	// StopWait bounds the wait for the reconciler to stop a database.
	StopWait time.Duration
}

const verifiedBy = "database upgrade"

func dataVolume(name string) string { return "db-" + name + "-data" }

// Busy reports a running major upgrade or restore of the same database.
func (d *DockerRuntime) Busy(ctx context.Context, name string) (string, error) {
	majors, err := d.Store.ListRunningMajorUpgrades(ctx)
	if err != nil {
		return "", fmt.Errorf("list major upgrades: %w", err)
	}
	for _, u := range majors {
		if u.DatabaseName == name {
			return "a major upgrade of this database is running", nil
		}
	}
	restores, err := d.Store.ListRestoreHistory(ctx, name)
	if err != nil {
		return "", fmt.Errorf("list restores: %w", err)
	}
	for _, r := range restores {
		if r.Status == store.BackupStatusRunning {
			return "a restore of this database is running", nil
		}
	}
	return "", nil
}

// RunBackup takes a logical backup to the database's own backup target.
func (d *DockerRuntime) RunBackup(ctx context.Context, backupID string, db store.DesiredDatabase) error {
	if d.Backups == nil {
		return errors.New("backups are not configured on this control plane (no master key set)")
	}
	return d.Backups.RunBackup(ctx, backupID, db.Name, db.Engine, database.ContainerName(db.Name), db.BackupTargetID)
}

// BackupStatus returns "" when the backup row does not exist.
func (d *DockerRuntime) BackupStatus(ctx context.Context, backupID string) (string, error) {
	h, err := d.Store.GetBackupHistory(ctx, backupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h.Status, nil
}

// RunVerify re-downloads the backup and checks its checksum and format.
func (d *DockerRuntime) RunVerify(ctx context.Context, verificationID, backupID, engine string) error {
	if d.Verifier == nil {
		return errors.New("backup verification is not configured on this control plane")
	}
	return d.Verifier.VerifyBackup(ctx, verificationID, backupID, engine, verifiedBy)
}

// VerifyStatus returns "" when the verification row does not exist.
func (d *DockerRuntime) VerifyStatus(ctx context.Context, backupID, verificationID string) (string, error) {
	list, err := d.Store.ListBackupVerifications(ctx, backupID, 50)
	if err != nil {
		return "", err
	}
	for _, v := range list {
		if v.ID == verificationID {
			return v.Status, nil
		}
	}
	return "", nil
}

// CheckImage pulls the target image by creating and removing a throwaway
// container, so a missing tag fails before the database stops.
func (d *DockerRuntime) CheckImage(ctx context.Context, engine, version string) error {
	ref := database.ImageRef(engine, version)
	id, err := d.Docker.Create(ctx, docker.ContainerSpec{Name: "dbupgrade-imgcheck-" + newID(""), Image: ref, Command: []string{"true"}})
	if err != nil {
		return fmt.Errorf("image %s: %w", ref, err)
	}
	return d.Docker.Remove(ctx, id, true)
}

// ImageDigest returns the image ID the database container runs now.
func (d *DockerRuntime) ImageDigest(ctx context.Context, name string) (string, error) {
	st, err := d.Docker.InspectByName(ctx, database.ContainerName(name))
	if err != nil {
		return "", err
	}
	if st == nil {
		return "", errors.New("the database container does not exist")
	}
	return st.ImageID, nil
}

// SnapshotData stops the database and copies its data volume into volume.
func (d *DockerRuntime) SnapshotData(ctx context.Context, name, volume string) error {
	if err := d.stop(ctx, name); err != nil {
		return err
	}
	if err := d.Docker.EnsureVolume(ctx, volume); err != nil {
		return fmt.Errorf("create snapshot volume: %w", err)
	}
	if err := d.WipeVolume(ctx, d.Docker, volume); err != nil {
		return err
	}
	return d.CopyVolume(ctx, d.Docker, dataVolume(name), volume)
}

// SetVersion is the set-version path: save the version, then let the
// reconciler recreate the container on the same data volume.
func (d *DockerRuntime) SetVersion(ctx context.Context, name, version string) error {
	db, err := d.Store.GetDesiredDatabase(ctx, name)
	if err != nil {
		return fmt.Errorf("load database: %w", err)
	}
	if db.Version != version {
		next := *db
		next.Version = version
		if err := d.Store.SaveDesiredDatabase(ctx, next); err != nil {
			return fmt.Errorf("save version %s: %w", version, err)
		}
	}
	return d.Resume(ctx, name)
}

// Resume clears the suspended flag so the reconciler starts the database.
func (d *DockerRuntime) Resume(ctx context.Context, name string) error {
	if err := d.Store.UpdateDatabaseSuspended(ctx, name, false); err != nil {
		return fmt.Errorf("resume database: %w", err)
	}
	d.nudge()
	return nil
}

// Health checks the container runs version's image, then the engine check.
func (d *DockerRuntime) Health(ctx context.Context, db store.DesiredDatabase, version string, deep bool) (bool, string, error) {
	st, err := d.Docker.InspectByName(ctx, database.ContainerName(db.Name))
	if err != nil {
		return false, "", err
	}
	if st == nil || !st.Running || !strings.HasSuffix(st.Image, ":"+database.ImageTag(db.Engine, version)) {
		return false, "", nil
	}
	if !deep {
		return true, "", nil
	}
	cmd, err := HealthCommand(db.Engine)
	if err != nil {
		return false, "", err
	}
	rc, err := d.Docker.Exec(ctx, database.ContainerName(db.Name), cmd)
	if err != nil {
		return false, "", err
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		return false, "", fmt.Errorf("health check: %w", err)
	}
	return true, parseHealthVersion(string(out)), nil
}

// RestoreSnapshot puts the snapshot back as the data volume on version.
func (d *DockerRuntime) RestoreSnapshot(ctx context.Context, name, volume, version string) error {
	if err := d.stop(ctx, name); err != nil {
		return err
	}
	if err := d.WipeVolume(ctx, d.Docker, dataVolume(name)); err != nil {
		return err
	}
	if err := d.CopyVolume(ctx, d.Docker, volume, dataVolume(name)); err != nil {
		return err
	}
	return d.SetVersion(ctx, name, version)
}

// ResetData empties the data volume and starts version on it fresh.
func (d *DockerRuntime) ResetData(ctx context.Context, name, version string) error {
	if err := d.stop(ctx, name); err != nil {
		return err
	}
	if err := d.WipeVolume(ctx, d.Docker, dataVolume(name)); err != nil {
		return err
	}
	return d.SetVersion(ctx, name, version)
}

// RestoreBackup restores the verified pre-upgrade backup.
func (d *DockerRuntime) RestoreBackup(ctx context.Context, restoreID, backupID string, db store.DesiredDatabase) error {
	if d.Restorer == nil {
		return errors.New("restores are not configured on this control plane")
	}
	return d.Restorer.RunRestore(ctx, restoreID, db.Name, backupID, db.Engine, database.ContainerName(db.Name))
}

// RemoveVolume deletes a snapshot volume.
func (d *DockerRuntime) RemoveVolume(ctx context.Context, volume string) error {
	return d.Docker.RemoveVolume(ctx, volume)
}

// stop suspends the database and waits for the reconciler to remove it.
func (d *DockerRuntime) stop(ctx context.Context, name string) error {
	if err := d.Store.UpdateDatabaseSuspended(ctx, name, true); err != nil {
		return fmt.Errorf("suspend database: %w", err)
	}
	d.nudge()
	wait := d.StopWait
	if wait <= 0 {
		wait = DefaultHealthTimeout
	}
	deadline := time.Now().Add(wait)
	for {
		st, err := d.Docker.InspectByName(ctx, database.ContainerName(name))
		if err != nil {
			return fmt.Errorf("inspect database container: %w", err)
		}
		if st == nil || !st.Running {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("the database was not stopped within %s", wait)
		}
		if err := sleepCtx(ctx, time.Second); err != nil {
			return err
		}
	}
}

func (d *DockerRuntime) nudge() {
	if d.Nudge != nil {
		d.Nudge()
	}
}
