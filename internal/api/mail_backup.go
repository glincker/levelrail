package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mail/mox"
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
		rt.mail.backup = &mailBackupConfig{dataDir: dataDir, recipients: recipients, identities: identities, secrets: secrets, jobs: map[string]*mailBackupJob{}}
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
	return &mailbackup.LocalSink{Dir: c.localDir(app), CapBytes: capB}, nil
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
	cfg, t, ok := rt.mailBackupSetup(w, r, r.URL.Query().Get("app"), false)
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
	m, err := mailbackup.Backup(ctx, mailbackup.BackupSpec{
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
	rep, err := mailbackup.Restore(r.Context(), mailbackup.RestoreSpec{Runner: runner, Sink: sink, ID: id, Identities: cfg.identities}, true)
	if err != nil {
		rt.mailRestoreError(w, err)
		return
	}
	out := mailRestoreResponse{App: t.svc.Name, DryRun: true, BackupID: id, MoxVersion: rep.Manifest.MoxVersion, Files: rep.Files, Bytes: rep.Bytes}
	if req.Confirm != id {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, mailRestoreStepUpApp+":"+t.svc.Name, "restore the mail server", "restoring the mail server"); !ok {
		return
	}
	dataVol := ""
	for _, v := range t.svc.Volumes {
		if v.ContainerPath == moxrun.DataDir {
			dataVol = v.Name
		}
	}
	if dataVol == "" {
		writeError(w, http.StatusConflict, "the mail app has no data volume at /mox/data, restore refused")
		return
	}
	job, started := cfg.start(t.svc.Name, "restore", id)
	if !started {
		writeError(w, http.StatusConflict, "a backup or restore is already running for this mail app")
		return
	}
	go rt.runMailRestore(context.WithoutCancel(r.Context()), cfg, t, sink, id, dataVol)
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

// runMailRestore stages the restore inside the data volume while mox runs,
// then stops mox only for the swap and waits for the admin API to answer.
func (rt *Router) runMailRestore(base context.Context, cfg *mailBackupConfig, t mailBackupTarget, sink *mailbackup.LocalSink, id, dataVol string) {
	ctx, cancel := context.WithTimeout(base, envMinutes("APP_MAIL_RESTORE_TIMEOUT_MINUTES", 60))
	defer cancel()
	app, cid := t.svc.Name, t.container.ID
	step := func(s string) { cfg.update(app, func(j *mailBackupJob) { j.Step = s }) }
	work := filepath.Join(cfg.dataDir, "mail-restore", app, id)
	err := func() error {
		_ = os.RemoveAll(work)
		defer func() { _ = os.RemoveAll(filepath.Dir(work)) }()
		step("extract")
		runner := moxrun.ExecRunner{RT: t.rt, ContainerID: cid}
		if _, err := mailbackup.Restore(ctx, mailbackup.RestoreSpec{Runner: runner, Sink: sink, ID: id, DestDir: work, Identities: cfg.identities}, false); err != nil {
			return err
		}
		step("stage")
		stage := moxrun.RestoreDir + "/" + id
		if err := moxrun.Push(ctx, t.rt, t.ar, cid, filepath.Join(work, "data"), stage); err != nil {
			return err
		}
		step("stop")
		if err := t.rt.Stop(ctx, cid, 30*time.Second); err != nil {
			return err
		}
		// Always try to bring mail back, even when the swap failed.
		defer func() { _ = t.rt.Start(context.WithoutCancel(ctx), cid) }()
		if st, err := t.rt.InspectByName(ctx, t.container.Name); err != nil || st == nil || st.Running {
			return errors.New("the mail container did not stay stopped, restore aborted before the swap")
		}
		step("swap")
		if err := moxrun.Swap(ctx, t.rt, t.svc.Image, dataVol, id); err != nil {
			return err
		}
		step("start")
		if err := t.rt.Start(ctx, cid); err != nil {
			return err
		}
		step("wait")
		wait := envMinutes("APP_MAIL_RESTORE_WAIT_MINUTES", 3)
		return moxrun.WaitRunning(ctx, t.rt, t.container.Name, wait, func(c docker.ContainerState) bool {
			ip := containerIP(c)
			return ip != "" && mox.AdminReachable(ctx, mox.AdminBaseURL(ip))
		})
	}()
	if err != nil {
		rt.logger.Warn("api: mail restore failed", slog.String("app", app), slog.String("backup", id), slog.String("error", err.Error()))
	}
	cfg.finish(app, id, err)
}
