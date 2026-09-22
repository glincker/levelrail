package backup

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// postgresContainerUID/GID is the official postgres image's fixed
// numeric "postgres" user/group; chowning by number also works against
// the generic alpine helper image, which has no "postgres" user.
const postgresContainerUID = 999
const postgresContainerGID = 999

// pitrConfFile is written fresh, then included from postgresql.auto.conf,
// on every restore: a separate file so a second restore can overwrite
// just this one rather than editing a file Postgres itself also writes
// to via ALTER SYSTEM.
const pitrConfFile = "postgresql.pitr.conf"

// PITRRestorer applies a base backup to dataVolumeName in place,
// configuring Postgres to replay WAL to targetTime and promote: the
// point-in-time-restore counterpart of Restorer. Unlike Restorer, this
// never touches a running container; dataVolumeName's container must
// already be stopped (PITRRunner's job).
type PITRRestorer interface {
	Restore(ctx context.Context, dataVolumeName string, baseBackup io.Reader, targetTime time.Time) error
}

// ContainerPITRRestorer is the real PITRRestorer: the wipe-then-extract-
// then-configure sequence inside a short-lived helper container, the
// same createVolumeHelper mechanism ContainerVolumeRestorer uses.
type ContainerPITRRestorer struct {
	Runtime docker.Runtime
}

// Restore implements PITRRestorer.
func (r *ContainerPITRRestorer) Restore(ctx context.Context, dataVolumeName string, baseBackup io.Reader, targetTime time.Time) error {
	id, err := createVolumeHelper(ctx, r.Runtime, dataVolumeName, "pitrrestore", false)
	if err != nil {
		return fmt.Errorf("backup: pitr restore volume %q: %w", dataVolumeName, err)
	}
	defer func() {
		_ = r.Runtime.Remove(context.Background(), id, true)
	}()

	rc, err := r.Runtime.ExecWithInput(ctx, id, pitrWipeAndExtractCmd, baseBackup)
	if err != nil {
		return fmt.Errorf("backup: pitr restore volume %q: extract base backup: %w", dataVolumeName, err)
	}
	drainErr := func() error {
		defer func() { _ = rc.Close() }()
		_, err := io.Copy(io.Discard, rc)
		return err
	}()
	if drainErr != nil {
		return fmt.Errorf("backup: pitr restore volume %q: extract base backup: %w", dataVolumeName, drainErr)
	}

	if err := r.writeRecoveryConfig(ctx, id, targetTime); err != nil {
		return fmt.Errorf("backup: pitr restore volume %q: %w", dataVolumeName, err)
	}
	return nil
}

// pitrWipeAndExtractCmd wipes volumeMountPath's contents (same three-
// glob clear as volumeRestoreCmd), extracts the base backup tar, then
// chowns to the postgres uid:gid as an explicit guarantee rather than an
// assumption about tar's own ownership handling.
var pitrWipeAndExtractCmd = []string{"sh", "-c", fmt.Sprintf(
	"cd %s && rm -rf -- ..?* .[!.]* *; tar -xf - -C %s && chown -R %d:%d %s && chmod 700 %s",
	volumeMountPath, volumeMountPath, postgresContainerUID, postgresContainerGID, volumeMountPath, volumeMountPath,
)}

// writeRecoveryConfig writes pitrConfFile, appends an include line to
// postgresql.auto.conf, and touches recovery.signal: the three files
// Postgres's archive recovery looks for at startup. Appending rather
// than deduplicating a prior restore's own include line is deliberate:
// Postgres tolerates the same file included twice (last wins, no
// error), a smaller risk than a more elaborate shell command on this
// path. Postgres deletes recovery.signal itself once recovery
// completes.
func (r *ContainerPITRRestorer) writeRecoveryConfig(ctx context.Context, helperID string, targetTime time.Time) error {
	// "+00", not "Z": the exact format Postgres's own now() prints and
	// accepts back for recovery_target_time.
	conf := fmt.Sprintf(
		"restore_command = 'cp %s/%%f %%p'\nrecovery_target_time = '%s+00'\nrecovery_target_action = 'promote'\nrecovery_target_timeline = 'latest'\n",
		postgresWALArchivePath, targetTime.UTC().Format("2006-01-02 15:04:05.999999"),
	)
	confPath := volumeMountPath + "/" + pitrConfFile
	autoConfPath := volumeMountPath + "/postgresql.auto.conf"
	signalPath := volumeMountPath + "/recovery.signal"
	cmd := []string{"sh", "-c", fmt.Sprintf(
		`cat > %s && echo "include '%s'" >> %s && touch %s && chown %d:%d %s %s %s`,
		confPath, pitrConfFile, autoConfPath, signalPath,
		postgresContainerUID, postgresContainerGID, confPath, signalPath, autoConfPath,
	)}

	rc, err := r.Runtime.ExecWithInput(ctx, helperID, cmd, strings.NewReader(conf))
	if err != nil {
		return fmt.Errorf("write recovery config: %w", err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("write recovery config: %w", err)
	}
	return nil
}
