package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mailbackup"
	"github.com/GLINCKER/levelrail/internal/mailbackup/moxrun"
	"github.com/GLINCKER/levelrail/internal/mailstorage"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	mailRestoreStepUpApp = "mail-restore"
	mailJobRunning       = "running"
	mailJobSucceeded     = "succeeded"
	mailJobFailed        = "failed"
)

// mailBackupConfig holds the encryption key from the instance backup sealer
// and the in-memory job table. Jobs do not survive a control plane restart.
type mailBackupConfig struct {
	dataDir    string
	recipients []age.Recipient
	identities []age.Identity
	secrets    mailstorage.SecretStore

	mu   sync.Mutex
	jobs map[string]*mailBackupJob
}

// WithMailBackup enables /api/v1/mail/backups and /api/v1/mail/storage. The
// age keys are the instance backup encryption key (backup.LoadSealer).
func WithMailBackup(dataDir string, recipients []age.Recipient, identities []age.Identity, secrets mailstorage.SecretStore) Option {
	return func(rt *Router) {
		cfg := &mailBackupConfig{dataDir: dataDir, recipients: recipients, identities: identities, secrets: secrets, jobs: map[string]*mailBackupJob{}}
		cfg.sweepHostLeftovers(time.Now())
		rt.mail.backup = cfg
	}
}

type mailBackupJob struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	State      string     `json:"state"`
	Step       string     `json:"step"`
	BackupID   string     `json:"backup_id,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      string     `json:"error,omitempty"`
	// Rollback is rolled_back or rollback_failed when a failed restore was undone.
	Rollback string `json:"rollback,omitempty"`
}

type mailBackupItem struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	StoredBytes int64     `json:"stored_bytes"`
	DataBytes   int64     `json:"data_bytes"`
	Files       int       `json:"files"`
	MoxVersion  string    `json:"mox_version"`
	Verified    bool      `json:"verified"`
	Where       string    `json:"where"`
}

type mailBackupListResponse struct {
	App       string           `json:"app"`
	Encrypted bool             `json:"encrypted"`
	Folder    string           `json:"folder"`
	CapBytes  int64            `json:"cap_bytes"`
	UsedBytes int64            `json:"used_bytes"`
	Backups   []mailBackupItem `json:"backups"`
	Job       *mailBackupJob   `json:"job,omitempty"`
}

type mailBackupRunRequest struct {
	App string `json:"app"`
}

type mailRestoreRequest struct {
	App string `json:"app"`
	// Confirm must equal the backup id for a real restore; anything else is a dry run.
	Confirm string       `json:"confirm"`
	StepUp  *stepUpProof `json:"step_up,omitempty"`
}

type mailRestoreResponse struct {
	App        string         `json:"app"`
	DryRun     bool           `json:"dry_run"`
	BackupID   string         `json:"backup_id"`
	MoxVersion string         `json:"mox_version"`
	Files      int            `json:"files"`
	Bytes      int64          `json:"bytes"`
	Job        *mailBackupJob `json:"job,omitempty"`
}

func (c *mailBackupConfig) localDir(app string) string {
	return filepath.Join(c.dataDir, "mail-backups", app)
}

func (c *mailBackupConfig) sink(app string) (*mailbackup.LocalSink, error) {
	capB, err := mailbackup.CapFromEnv(os.Getenv)
	if err != nil {
		return nil, err
	}
	keep, err := mailbackup.MinKeepFromEnv(os.Getenv)
	if err != nil {
		return nil, err
	}
	return &mailbackup.LocalSink{Dir: c.localDir(app), CapBytes: capB, MinKeep: keep}, nil
}

func (c *mailBackupConfig) current(app string) *mailBackupJob {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j := c.jobs[app]; j != nil {
		cp := *j
		return &cp
	}
	return nil
}

// start registers a job, refusing while another one runs for the app.
func (c *mailBackupConfig) start(app, kind, backupID string) (*mailBackupJob, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j := c.jobs[app]; j != nil && j.State == mailJobRunning {
		return nil, false
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	j := &mailBackupJob{ID: hex.EncodeToString(b), Kind: kind, State: mailJobRunning, Step: "queued", BackupID: backupID, StartedAt: time.Now().UTC()}
	c.jobs[app] = j
	return j, true
}

func (c *mailBackupConfig) update(app string, fn func(j *mailBackupJob)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j := c.jobs[app]; j != nil {
		fn(j)
	}
}

func (c *mailBackupConfig) finish(app string, backupID string, err error) {
	c.update(app, func(j *mailBackupJob) {
		now := time.Now().UTC()
		j.FinishedAt, j.State = &now, mailJobSucceeded
		if backupID != "" {
			j.BackupID = backupID
		}
		if err != nil {
			j.State, j.Error = mailJobFailed, err.Error()
		}
	})
}

func envMinutes(key string, def int) time.Duration {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return time.Duration(n) * time.Minute
	}
	return time.Duration(def) * time.Minute
}

// mailBackupTarget is the resolved engine container plus the runtime that reaches it.
type mailBackupTarget struct {
	svc       store.DesiredService
	container docker.ContainerState
	rt        docker.Runtime
	ar        docker.ArchiveRuntime
}

// mailBackupSetup resolves the app, the engine container (platform identity
// only, via mailEngineContainer) and the backup config, writing any error.
func (rt *Router) mailBackupSetup(w http.ResponseWriter, r *http.Request, app string, needEngine bool) (*mailBackupConfig, mailBackupTarget, bool) {
	var t mailBackupTarget
	cfg := rt.mail.backup
	if cfg == nil {
		writeError(w, http.StatusNotImplemented, "mail backups are not configured on this control plane")
		return nil, t, false
	}
	svc, found, err := rt.pickMailApp(r, app)
	if err != nil {
		rt.internalError(w, "api: mail backup: list services failed", err)
		return nil, t, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "no mail app is installed, install the mail-mox template first")
		return nil, t, false
	}
	if !mailstorage.ValidServiceID(svc.Name) {
		writeError(w, http.StatusBadRequest, "the mail app name cannot be used as a backup folder")
		return nil, t, false
	}
	t.svc = svc
	if strings.HasPrefix(svc.MaintenanceHold, mailRestoreHoldPrefix) {
		if j := cfg.current(svc.Name); j == nil || j.State != mailJobRunning {
			rt.startMailRecovery(r.Context(), cfg, svc)
		}
		writeError(w, http.StatusConflict, "a mail restore holds this app; it is finishing or being rolled back, check the job in the backup list")
		return nil, t, false
	}
	if !needEngine {
		return cfg, t, true
	}
	c, err := rt.mailEngineContainer(r.Context(), svc)
	if err != nil || !c.Running {
		writeError(w, http.StatusServiceUnavailable, "the mail container is not running or does not match this instance; check levelrail-cli mail status")
		return nil, t, false
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "container exec is not configured on this control plane")
		return nil, t, false
	}
	runtime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "the node that runs the mail app is not reachable")
		return nil, t, false
	}
	ar, ok := runtime.(docker.ArchiveRuntime)
	if !ok {
		writeError(w, http.StatusNotImplemented, "the mail node's runtime cannot copy files out of containers")
		return nil, t, false
	}
	t.container, t.rt, t.ar = c, runtime, ar
	return cfg, t, true
}

// handleListMailBackups handles GET /api/v1/mail/backups.
func (rt *Router) handleListMailBackups(w http.ResponseWriter, r *http.Request) {
	cfg, t, ok := rt.mailBackupSetupOrHeld(w, r, r.URL.Query().Get("app"))
	if !ok {
		return
	}
	sink, err := cfg.sink(t.svc.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ms, err := mailbackup.List(r.Context(), sink)
	if err != nil {
		rt.internalError(w, "api: mail backup: list", err)
		return
	}
	used, _ := sink.UsedBytes()
	out := mailBackupListResponse{
		App: t.svc.Name, Encrypted: len(cfg.recipients) > 0, Folder: sink.Dir,
		CapBytes: sink.CapBytes, UsedBytes: used, Backups: []mailBackupItem{}, Job: cfg.current(t.svc.Name),
	}
	for _, m := range ms {
		if !moxrun.ValidID(m.ID) {
			continue
		}
		var stored int64
		// The name is rebuilt from the validated id, never taken from the manifest.
		if fi, err := os.Stat(filepath.Join(sink.Dir, "mail-"+m.ID+".tar.zst.age")); err == nil { //nolint:gosec // id matches moxrun.ValidID
			stored = fi.Size()
		}
		out.Backups = append(out.Backups, mailBackupItem{
			ID: m.ID, CreatedAt: m.CreatedAt, StoredBytes: stored, DataBytes: m.Bytes, Files: m.Files,
			MoxVersion: m.MoxVersion, Verified: m.Verified, Where: "local",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRunMailBackup handles POST /api/v1/mail/backups. It answers 202 at
// once; the caller polls GET /api/v1/mail/backups for the job.
func (rt *Router) handleRunMailBackup(w http.ResponseWriter, r *http.Request) {
	var req mailBackupRunRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, t, ok := rt.mailBackupSetup(w, r, req.App, true)
	if !ok {
		return
	}
	if len(cfg.recipients) == 0 {
		writeError(w, http.StatusConflict, "mail backups are always encrypted, but the instance backup encryption key is off (APP_BACKUP_ENCRYPTION)")
		return
	}
	sink, err := cfg.sink(t.svc.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job, started := cfg.start(t.svc.Name, "backup", "")
	if !started {
		writeError(w, http.StatusConflict, "a backup or restore is already running for this mail app")
		return
	}
	go rt.runMailBackup(context.WithoutCancel(r.Context()), cfg, t, sink, job.ID)
	writeJSON(w, http.StatusAccepted, job)
}

func (rt *Router) runMailBackup(base context.Context, cfg *mailBackupConfig, t mailBackupTarget, sink *mailbackup.LocalSink, jobID string) {
	ctx, cancel := context.WithTimeout(base, envMinutes("APP_MAIL_BACKUP_TIMEOUT_MINUTES", 60))
	defer cancel()
	app, cid := t.svc.Name, t.container.ID
	vols := mailVolumes(t.svc)
	cfg.sweepHostLeftovers(time.Now())
	sweepVolume(ctx, t.rt, cid, vols.Config != "")
	var fetchConfig func(context.Context, string) error
	if vols.Config != "" {
		fetchConfig = func(ctx context.Context, host string) error {
			return moxrun.Fetch(ctx, t.ar, cid, moxrun.ConfigDir, host, sink.CapBytes)
		}
	}
	m, err := mailbackup.Backup(ctx, mailbackup.BackupSpec{
		FetchConfig:      fetchConfig,
		Runner:           moxrun.ExecRunner{RT: t.rt, ContainerID: cid},
		Sink:             sink,
		StagingHost:      filepath.Join(cfg.dataDir, "mail-backup-staging", app),
		StagingContainer: moxrun.StagingDir,
		Recipients:       cfg.recipients,
		Fetch: func(ctx context.Context, cont, host string) error {
			return moxrun.Fetch(ctx, t.ar, cid, cont, host, sink.CapBytes)
		},
		Cleanup: func(ctx context.Context, cont string) { _ = moxrun.RemoveStaging(ctx, t.rt, cid, cont) },
		Progress: func(step string) {
			cfg.update(app, func(j *mailBackupJob) { j.Step = step })
		},
	})
	if err != nil {
		rt.logger.Warn("api: mail backup failed", slog.String("app", app), slog.String("job", jobID), slog.String("error", err.Error()))
	}
	cfg.finish(app, m.ID, err)
}

// handleRestoreMailBackup handles POST /api/v1/mail/backups/{id}/restore.
// Without confirm equal to the id it is a dry run that writes nothing.
func (rt *Router) handleRestoreMailBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !moxrun.ValidID(id) {
		writeError(w, http.StatusBadRequest, "invalid backup id")
		return
	}
	var req mailRestoreRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<14); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, t, ok := rt.mailBackupSetup(w, r, req.App, true)
	if !ok {
		return
	}
	if len(cfg.identities) == 0 {
		writeError(w, http.StatusConflict, "this control plane has no backup decryption key")
		return
	}
	sink, err := cfg.sink(t.svc.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	runner := moxrun.ExecRunner{RT: t.rt, ContainerID: t.container.ID}
	spec := mailbackup.RestoreSpec{Runner: runner, Sink: sink, ID: id, Identities: cfg.identities}
	if req.Confirm != id {
		rep, err := mailbackup.Restore(r.Context(), spec, true)
		if err != nil {
			rt.mailRestoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mailRestoreResponse{App: t.svc.Name, DryRun: true, BackupID: id, MoxVersion: rep.Manifest.MoxVersion, Files: rep.Files, Bytes: rep.Bytes})
		return
	}
	// A confirmed restore decrypts once, in the job: here only the manifest and version guard run.
	m, err := mailbackup.Check(r.Context(), spec)
	if err != nil {
		rt.mailRestoreError(w, err)
		return
	}
	out := mailRestoreResponse{App: t.svc.Name, BackupID: id, MoxVersion: m.MoxVersion, Files: m.Files, Bytes: m.Bytes}
	holder, ok := rt.apps.(mailHolder)
	if !ok {
		writeError(w, http.StatusNotImplemented, "this control plane cannot hold the mail app during a restore, restore refused")
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, mailRestoreStepUpApp+":"+t.svc.Name, "restore the mail server", "restoring the mail server"); !ok {
		return
	}
	vols := mailVolumes(t.svc)
	if vols.Data == "" {
		writeError(w, http.StatusConflict, "the mail app has no data volume at /mox/data, restore refused")
		return
	}
	job, started := cfg.start(t.svc.Name, "restore", id)
	if !started {
		writeError(w, http.StatusConflict, "a backup or restore is already running for this mail app")
		return
	}
	go rt.runMailRestore(context.WithoutCancel(r.Context()), cfg, t, sink, holder, id, vols)
	out.DryRun, out.Job = false, job
	writeJSON(w, http.StatusAccepted, out)
}

func (rt *Router) mailRestoreError(w http.ResponseWriter, err error) {
	var dg *mailbackup.DowngradeError
	var ck *mailbackup.ChecksumError
	switch {
	case errors.As(err, &dg):
		writeError(w, http.StatusConflict, dg.Error())
	case errors.As(err, &ck):
		writeError(w, http.StatusUnprocessableEntity, "the backup archive does not match its manifest checksum")
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "backup not found")
	default:
		rt.logger.Warn("api: mail restore check failed", slog.String("error", err.Error()))
		writeError(w, http.StatusUnprocessableEntity, "the backup could not be read or decrypted")
	}
}

// runMailRestore extracts and stages the restore on the mail volumes while mox
// runs, verifies it with mox verifydata, then swaps it in under a persisted
// hold with rollback (mailSwap). Plaintext copies are removed on every path.
func (rt *Router) runMailRestore(base context.Context, cfg *mailBackupConfig, t mailBackupTarget, sink *mailbackup.LocalSink, holder mailHolder, id string, vols moxrun.Volumes) {
	ctx, cancel := context.WithTimeout(base, envMinutes("APP_MAIL_RESTORE_TIMEOUT_MINUTES", 60))
	defer cancel()
	app, cid := t.svc.Name, t.container.ID
	step := func(s string) { cfg.update(app, func(j *mailBackupJob) { j.Step = s }) }
	work := filepath.Join(cfg.dataDir, "mail-restore", app, id)
	rollback := ""
	err := func() error {
		_ = os.RemoveAll(work)
		defer func() { _ = os.RemoveAll(filepath.Dir(work)) }()
		cfg.sweepHostLeftovers(time.Now())
		sweepVolume(ctx, t.rt, cid, vols.Config != "")
		step("extract")
		runner := moxrun.ExecRunner{RT: t.rt, ContainerID: cid}
		if _, err := mailbackup.Restore(ctx, mailbackup.RestoreSpec{Runner: runner, Sink: sink, ID: id, DestDir: work, Identities: cfg.identities}, false); err != nil {
			return err
		}
		withConfig := false
		if fi, err := os.Stat(filepath.Join(work, "config")); err == nil && fi.IsDir() && vols.Config != "" {
			withConfig = true
		}
		roots := rootsFor(vols, withConfig)
		swapped := false
		defer func() {
			if !swapped {
				for _, r := range roots {
					_ = moxrun.RemoveStaging(context.WithoutCancel(ctx), t.rt, cid, moxrun.StageDir(r.Dir, id))
				}
			}
		}()
		step("stage")
		for _, r := range roots {
			if err := moxrun.Push(ctx, t.rt, t.ar, cid, filepath.Join(work, r.Sub), moxrun.StageDir(r.Dir, id)); err != nil {
				return err
			}
		}
		_ = os.RemoveAll(work)
		step("verify")
		if _, err := runner.Run(ctx, "verifydata", moxrun.StageDir(moxrun.DataDir, id)+"/data"); err != nil {
			return fmt.Errorf("mox verifydata on the staged restore: %w", err)
		}
		swapped = true
		s := &mailSwap{rt: t.rt, holder: holder, t: t, vols: vols, roots: roots, step: step, logger: rt.logger}
		var err error
		rollback, err = s.run(ctx, id)
		return err
	}()
	if err != nil {
		rt.logger.Warn("api: mail restore failed", slog.String("app", app), slog.String("backup", id), slog.String("rollback", rollback), slog.String("error", err.Error()))
	}
	cfg.finish(app, id, err)
	cfg.update(app, func(j *mailBackupJob) { j.Rollback = rollback })
}

// mailBackupSetupOrHeld is mailBackupSetup for the list call, which still
// answers while a hold is being recovered so the dashboard can show the job.
func (rt *Router) mailBackupSetupOrHeld(w http.ResponseWriter, r *http.Request, app string) (*mailBackupConfig, mailBackupTarget, bool) {
	var t mailBackupTarget
	if rt.mail.backup != nil {
		if svc, found, err := rt.pickMailApp(r, app); err == nil && found && strings.HasPrefix(svc.MaintenanceHold, mailRestoreHoldPrefix) && mailstorage.ValidServiceID(svc.Name) {
			cfg := rt.mail.backup
			if j := cfg.current(svc.Name); j == nil || j.State != mailJobRunning {
				rt.startMailRecovery(r.Context(), cfg, svc)
			}
			t.svc = svc
			return cfg, t, true
		}
	}
	return rt.mailBackupSetup(w, r, app, false)
}

// startMailRecovery runs mailSwap.recover as a job for an app whose persisted
// hold outlived its restore job (a control plane restart or crash).
func (rt *Router) startMailRecovery(ctx context.Context, cfg *mailBackupConfig, svc store.DesiredService) {
	holder, ok := rt.apps.(mailHolder)
	if !ok || rt.execRuntime == nil {
		return
	}
	job, started := cfg.start(svc.Name, "recover", strings.TrimPrefix(svc.MaintenanceHold, mailRestoreHoldPrefix))
	if !started {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), envMinutes("APP_MAIL_RESTORE_TIMEOUT_MINUTES", 60))
		defer cancel()
		rollback, err := func() (string, error) {
			c, err := rt.mailEngineContainer(ctx, svc)
			if err != nil {
				return "", fmt.Errorf("find the held mail container: %w", err)
			}
			runtime, err := rt.execRuntime(svc.NodeID)
			if err != nil {
				return "", fmt.Errorf("reach the mail node: %w", err)
			}
			step := func(s string) { cfg.update(svc.Name, func(j *mailBackupJob) { j.Step = s }) }
			s := &mailSwap{rt: runtime, holder: holder, t: mailBackupTarget{svc: svc, container: c, rt: runtime}, vols: mailVolumes(svc), roots: rootsFor(mailVolumes(svc), true), step: step, logger: rt.logger}
			return s.recover(ctx)
		}()
		if err != nil {
			rt.logger.Warn("api: mail restore recovery", slog.String("app", svc.Name), slog.String("job", job.ID), slog.String("rollback", rollback), slog.String("error", err.Error()))
		}
		cfg.finish(svc.Name, "", err)
		cfg.update(svc.Name, func(j *mailBackupJob) { j.Rollback = rollback })
	}()
}
