package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Drill stages, recorded on store.BackupDrill.Stage.
const (
	DrillStageObject   = "object"
	DrillStageDownload = "download"
	DrillStageRestore  = "restore"
	DrillStageContent  = "content"
	DrillStageDone     = "done"
)

// DrillStore is the store surface a drill needs.
type DrillStore interface {
	backupResolver
	StartBackupDrill(ctx context.Context, d store.BackupDrill) error
	FinishBackupDrill(ctx context.Context, d store.BackupDrill) error
}

// VolumeRemover deletes a Docker volume by name.
type VolumeRemover interface {
	RemoveVolume(ctx context.Context, name string) error
}

// DatabaseInfo resolves a database's engine and version for a drill.
type DatabaseInfo func(ctx context.Context, databaseName string) (engine, version string, err error)

// DrillRunner restores a stored backup into a scratch resource, validates it
// and destroys the scratch resource, recording every step.
type DrillRunner struct {
	Store      DrillStore
	Secrets    SecretsResolver
	Downloader Downloader
	Prober     ObjectProber
	Identities []age.Identity

	// Volume drills.
	Restorer VolumeRestorer
	Archiver VolumeArchiver
	Volumes  VolumeCreator
	Remover  VolumeRemover

	// Database drills. Any nil leaves databases at integrity-only checks.
	Runtime    docker.Runtime
	DBRestorer Restorer
	DBInfo     DatabaseInfo
	ImageFor   func(engine, version string) string

	// OnFailure is called once for every failed drill.
	OnFailure func(ctx context.Context, d store.BackupDrill)
	Logger    *slog.Logger
	Now       func() time.Time
}

func (r *DrillRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *DrillRunner) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

// RunDrill executes one drill of backupHistoryID. The drill row is always
// finished; the returned drill carries the outcome. A failed drill returns a
// non-nil error describing it.
func (r *DrillRunner) RunDrill(ctx context.Context, drillID, backupHistoryID, trigger string) (store.BackupDrill, error) {
	started := r.now()
	d := store.BackupDrill{
		ID:              drillID,
		BackupHistoryID: backupHistoryID,
		Trigger:         trigger,
		StartedAt:       started.UTC().Format(time.RFC3339),
	}
	bh, err := r.Store.GetBackupHistory(ctx, backupHistoryID)
	if err != nil {
		return d, fmt.Errorf("get backup %q: %w", backupHistoryID, err)
	}
	d.ResourceKind, d.DatabaseName, d.ServiceName, d.VolumeName, d.TargetID = bh.ResourceKind, bh.DatabaseName, bh.ServiceName, bh.VolumeName, bh.TargetID
	if err := r.Store.StartBackupDrill(ctx, d); err != nil {
		return d, fmt.Errorf("start drill: %w", err)
	}

	runErr := r.execute(ctx, &d, bh)

	d.DurationMS = r.now().Sub(started).Milliseconds()
	d.FinishedAt = r.now().UTC().Format(time.RFC3339)
	if runErr != nil {
		d.Status = store.DrillStatusFailed
		d.Error = runErr.Error()
	} else {
		d.Status = store.DrillStatusPassed
		d.Stage = DrillStageDone
	}
	if err := r.Store.FinishBackupDrill(ctx, d); err != nil {
		return d, fmt.Errorf("finish drill %q: %w", drillID, err)
	}
	if runErr != nil {
		r.log().Error("backup: restore drill failed", slog.String("drill_id", drillID), slog.String("backup_id", backupHistoryID), slog.String("stage", d.Stage), slog.String("error", runErr.Error()))
		if r.OnFailure != nil {
			r.OnFailure(ctx, d)
		}
		return d, fmt.Errorf("restore drill %q failed at %s: %w", drillID, d.Stage, runErr)
	}
	return d, nil
}

func (r *DrillRunner) execute(ctx context.Context, d *store.BackupDrill, bh store.BackupHistory) error {
	if bh.Status != store.BackupStatusSucceeded {
		d.Stage = DrillStageObject
		return fmt.Errorf("backup has status %q: only a succeeded backup can be drilled", bh.Status)
	}
	dest, err := resolveTargetDestination(ctx, r.Store, r.Secrets, bh.TargetID)
	if err != nil {
		d.Stage = DrillStageObject
		return err
	}

	d.Stage = DrillStageObject
	if r.Prober != nil {
		info, err := r.Prober.Head(ctx, dest, bh.ObjectKey)
		if err != nil {
			return fmt.Errorf("check object: %w", err)
		}
		if state, reason := CheckObject(info, bh); state != ObjectOK {
			return errors.New(reason)
		}
	}
	d.ObjectOK = true

	if bh.ResourceKind == store.BackupResourceKindVolume {
		if isSQLiteObject(bh) {
			return r.drillSQLite(ctx, d, dest, bh)
		}
		return r.drillVolume(ctx, d, dest, bh)
	}
	return r.drillDatabase(ctx, d, dest, bh)
}

func isSQLiteObject(bh store.BackupHistory) bool {
	return strings.HasSuffix(strings.TrimSuffix(strings.TrimSuffix(bh.ObjectKey, ".age"), ".zst"), ".db")
}

// hashingStream wraps a download so everything read through it is hashed and
// counted, and Drain finishes reading whatever the decoder left behind.
type hashingStream struct {
	rc   io.ReadCloser
	hash interface {
		io.Writer
		Sum([]byte) []byte
	}
	n int64
}

func (h *hashingStream) Read(p []byte) (int, error) {
	n, err := h.rc.Read(p)
	if n > 0 {
		h.n += int64(n)
		_, _ = h.hash.Write(p[:n])
	}
	return n, err
}

func (h *hashingStream) drain() error {
	_, err := io.Copy(io.Discard, h)
	return err
}

func (r *DrillRunner) openObject(ctx context.Context, dest Destination, bh store.BackupHistory) (*hashingStream, error) {
	rc, err := r.Downloader.Download(ctx, dest, bh.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("download %q: %w", bh.ObjectKey, err)
	}
	return &hashingStream{rc: rc, hash: sha256.New()}, nil
}

func (h *hashingStream) checksum() string { return hex.EncodeToString(h.hash.Sum(nil)) }

func (r *DrillRunner) checkChecksum(d *store.BackupDrill, hs *hashingStream, bh store.BackupHistory) error {
	if bh.ChecksumSHA256 != "" && hs.checksum() != bh.ChecksumSHA256 {
		return errors.New("checksum of the stored object does not match the one recorded at backup time: the object was corrupted or replaced")
	}
	if bh.SizeBytes != 0 && hs.n != bh.SizeBytes {
		return fmt.Errorf("stored object is %d bytes but %d were uploaded", hs.n, bh.SizeBytes)
	}
	d.ChecksumOK = true
	return nil
}

func (r *DrillRunner) drillVolume(ctx context.Context, d *store.BackupDrill, dest Destination, bh store.BackupHistory) (err error) {
	if r.Restorer == nil || r.Archiver == nil || r.Volumes == nil || r.Remover == nil {
		return errors.New("volume drills are not configured on this control plane")
	}
	scratch := "drill-" + d.ID

	d.Stage = DrillStageDownload
	hs, err := r.openObject(ctx, dest, bh)
	if err != nil {
		return err
	}
	defer func() { _ = hs.rc.Close() }()

	if err := r.Volumes.EnsureVolume(ctx, scratch); err != nil {
		return fmt.Errorf("create scratch volume: %w", err)
	}
	defer func() {
		if rerr := r.Remover.RemoveVolume(context.WithoutCancel(ctx), scratch); rerr != nil {
			r.log().Error("backup: remove drill scratch volume failed", slog.String("volume", scratch), slog.String("error", rerr.Error()))
			if err == nil {
				err = fmt.Errorf("drill passed but scratch volume %q could not be removed: %w", scratch, rerr)
			}
		}
	}()

	plain, err := OpenSealed(hs, bh.Codec, r.Identities)
	if err != nil {
		return err
	}
	defer func() { _ = plain.Close() }()
	archive, wait := manifestTee(plain)

	d.Stage = DrillStageRestore
	rerr := r.Restorer.Restore(ctx, scratch, archive)
	drainErr := hs.drain()
	srcMan, manErr := wait()
	if rerr != nil {
		return fmt.Errorf("restore into scratch volume: %w", rerr)
	}
	if drainErr != nil {
		return fmt.Errorf("read rest of stored object: %w", drainErr)
	}
	d.Stage = DrillStageDownload
	if err := r.checkChecksum(d, hs, bh); err != nil {
		return err
	}
	if manErr != nil {
		return fmt.Errorf("archive is not a readable tar: %w", manErr)
	}
	d.RestoreOK = true

	d.Stage = DrillStageContent
	rc, err := r.Archiver.Archive(ctx, scratch)
	if err != nil {
		return fmt.Errorf("read back scratch volume: %w", err)
	}
	got, gerr := ComputeManifest(rc)
	_ = rc.Close()
	if gerr != nil {
		return fmt.Errorf("read back scratch volume: %w", gerr)
	}
	d.Files, d.Bytes = got.Files, got.Bytes
	if bh.ManifestDigest != "" && srcMan.Digest != bh.ManifestDigest {
		return errors.New("archive content does not match the manifest recorded at backup time")
	}
	if got.Digest != srcMan.Digest {
		return fmt.Errorf("restored volume differs from the archive: archive has %d files and %d bytes, restored volume has %d files and %d bytes", srcMan.Files, srcMan.Bytes, got.Files, got.Bytes)
	}
	d.ContentOK = true
	return nil
}

var sqliteMagic = []byte("SQLite format 3\x00")

func (r *DrillRunner) drillSQLite(ctx context.Context, d *store.BackupDrill, dest Destination, bh store.BackupHistory) error {
	d.Stage = DrillStageDownload
	hs, err := r.openObject(ctx, dest, bh)
	if err != nil {
		return err
	}
	defer func() { _ = hs.rc.Close() }()
	plain, err := OpenSealed(hs, bh.Codec, r.Identities)
	if err != nil {
		return err
	}
	defer func() { _ = plain.Close() }()

	head := make([]byte, len(sqliteMagic))
	if _, err := io.ReadFull(plain, head); err != nil {
		return fmt.Errorf("read sqlite header: %w", err)
	}
	if _, err := io.Copy(io.Discard, plain); err != nil {
		return fmt.Errorf("read sqlite snapshot: %w", err)
	}
	if err := hs.drain(); err != nil {
		return err
	}
	if err := r.checkChecksum(d, hs, bh); err != nil {
		return err
	}
	d.Stage = DrillStageContent
	if !bytes.Equal(head, sqliteMagic) {
		return errors.New("snapshot does not start with the SQLite file header")
	}
	d.RestoreOK, d.ContentOK, d.Bytes = true, true, hs.n
	return nil
}

func (r *DrillRunner) drillDatabase(ctx context.Context, d *store.BackupDrill, dest Destination, bh store.BackupHistory) (err error) {
	d.Stage = DrillStageDownload
	hs, err := r.openObject(ctx, dest, bh)
	if err != nil {
		return err
	}
	defer func() { _ = hs.rc.Close() }()

	engine, version := "", ""
	if r.DBInfo != nil {
		if e, v, ierr := r.DBInfo(ctx, bh.DatabaseName); ierr == nil {
			engine, version = e, v
		}
	}
	scratchable := r.Runtime != nil && r.DBRestorer != nil && r.ImageFor != nil && scratchEnv(engine, bh.DatabaseName) != nil

	header := newHeaderCapture(headerCaptureBytes)
	tail := &tailCapture{maxBytes: tailCaptureBytes}

	if !scratchable {
		if _, err := io.Copy(io.MultiWriter(header, tail), hs); err != nil {
			return fmt.Errorf("read stored object: %w", err)
		}
		if err := r.checkChecksum(d, hs, bh); err != nil {
			return err
		}
		d.Stage = DrillStageContent
		if ok, why := validateFormat(engine, header.bytes, tail.bytes(), hs.n); !ok {
			return errors.New(why)
		}
		d.ContentOK, d.Bytes = true, hs.n
		return nil
	}

	name := "drill-" + d.ID
	id, err := r.Runtime.Create(ctx, docker.ContainerSpec{Name: name, Image: r.ImageFor(engine, version), Env: scratchEnv(engine, bh.DatabaseName)})
	if err != nil {
		return fmt.Errorf("create scratch %s container: %w", engine, err)
	}
	defer func() {
		if rerr := r.Runtime.Remove(context.WithoutCancel(ctx), id, true); rerr != nil && err == nil {
			err = fmt.Errorf("drill passed but scratch container %q could not be removed: %w", name, rerr)
		}
	}()
	if err := r.Runtime.Start(ctx, id); err != nil {
		return fmt.Errorf("start scratch %s container: %w", engine, err)
	}

	d.Stage = DrillStageRestore
	rerr := r.DBRestorer.Restore(ctx, engine, name, io.TeeReader(hs, io.MultiWriter(header, tail)))
	drainErr := hs.drain()
	if rerr != nil {
		return fmt.Errorf("restore into scratch %s container: %w", engine, rerr)
	}
	if drainErr != nil {
		return fmt.Errorf("read rest of stored object: %w", drainErr)
	}
	if err := r.checkChecksum(d, hs, bh); err != nil {
		return err
	}
	d.RestoreOK = true

	d.Stage = DrillStageContent
	if ok, why := validateFormat(engine, header.bytes, tail.bytes(), hs.n); !ok {
		return errors.New(why)
	}
	tables, err := r.countTables(ctx, name, engine)
	if err != nil {
		return fmt.Errorf("query restored database: %w", err)
	}
	d.Files, d.Bytes, d.ContentOK = tables, hs.n, true
	return nil
}

func scratchEnv(engine, dbName string) map[string]string {
	switch engine {
	case store.EnginePostgres:
		return map[string]string{"POSTGRES_USER": dbName, "POSTGRES_PASSWORD": "drill", "POSTGRES_DB": dbName}
	case store.EngineMySQL:
		return map[string]string{"MYSQL_ROOT_PASSWORD": "drill", "MYSQL_DATABASE": dbName}
	case store.EngineMariaDB:
		return map[string]string{"MARIADB_ROOT_PASSWORD": "drill", "MARIADB_DATABASE": dbName}
	}
	return nil
}

var tableCountCmds = map[string][]string{
	store.EnginePostgres: {"sh", "-c", `psql --no-password -U "$POSTGRES_USER" -d "$POSTGRES_USER" -tA -c "select count(*) from information_schema.tables where table_schema = 'public'"`},
	store.EngineMySQL:    {"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -N -e "select count(*) from information_schema.tables where table_schema = '$MYSQL_DATABASE'"`},
	store.EngineMariaDB:  {"sh", "-c", `mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -N -e "select count(*) from information_schema.tables where table_schema = '$MARIADB_DATABASE'"`},
}

func (r *DrillRunner) countTables(ctx context.Context, container, engine string) (int64, error) {
	rc, err := r.Runtime.Exec(ctx, container, tableCountCmds[engine])
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(io.LimitReader(rc, 4096))
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(lastLine(string(out))), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected output %q", strings.TrimSpace(string(out)))
	}
	return n, nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
