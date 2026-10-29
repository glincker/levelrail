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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envPredeploySnapshotTimeout = "APP_PREDEPLOY_SNAPSHOT_TIMEOUT"
	envPredeploySnapshotMaxMB   = "APP_PREDEPLOY_SNAPSHOT_MAX_SIZE_MB"
	envPredeploySnapshotKeep    = "APP_PREDEPLOY_SNAPSHOT_KEEP"

	defaultPredeploySnapshotTimeout = 2 * time.Minute
	defaultPredeploySnapshotMaxMB   = 2048
	defaultPredeploySnapshotKeep    = 3
)

// PredeploySnapshotStore is the store surface the pre-deploy snapshot feature needs.
type PredeploySnapshotStore interface {
	GetPredeploySnapshotSettings(ctx context.Context, app string) (store.PredeploySnapshotSettings, error)
	SetPredeploySnapshotSettings(ctx context.Context, app string, s store.PredeploySnapshotSettings) error
	SavePredeploySnapshot(ctx context.Context, p store.PredeploySnapshot) error
	AttachPredeploySnapshotsToDeploy(ctx context.Context, ids []string, deployID string) error
	SetPredeploySnapshotRestore(ctx context.Context, id, restoredDatabase, status, errMsg string) error
	ListPredeploySnapshots(ctx context.Context, app string) ([]store.PredeploySnapshot, error)
	ListPredeploySnapshotsForDeploy(ctx context.Context, app, deployID string) ([]store.PredeploySnapshot, error)
	DeletePredeploySnapshot(ctx context.Context, id string) error
}

func predeploySnapshotTimeout() time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(envPredeploySnapshotTimeout))); err == nil && d > 0 {
		return d
	}
	return defaultPredeploySnapshotTimeout
}

func predeploySnapshotMaxBytes() int64 {
	mb := int64(defaultPredeploySnapshotMaxMB)
	if n, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(envPredeploySnapshotMaxMB)), 10, 64); err == nil && n > 0 {
		mb = n
	}
	return mb << 20
}

func predeploySnapshotKeep() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envPredeploySnapshotKeep))); err == nil && n > 0 {
		return n
	}
	return defaultPredeploySnapshotKeep
}

type predeploySnapshotSettingsResource struct {
	Mode     string `json:"mode"`
	TargetID string `json:"target_id"`
}

type predeploySnapshotResource struct {
	ID               string `json:"id"`
	DeployID         string `json:"deploy_id,omitempty"`
	PriorDeployID    string `json:"prior_deploy_id,omitempty"`
	DatabaseName     string `json:"database_name"`
	Engine           string `json:"engine,omitempty"`
	BackupID         string `json:"backup_id,omitempty"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	SizeBytes        int64  `json:"size_bytes"`
	CreatedAt        string `json:"created_at"`
	RestoredDatabase string `json:"restored_database,omitempty"`
	RestoreStatus    string `json:"restore_status,omitempty"`
	RestoreError     string `json:"restore_error,omitempty"`
}

func toPredeploySnapshotResource(p store.PredeploySnapshot) predeploySnapshotResource {
	return predeploySnapshotResource{
		ID: p.ID, DeployID: p.DeployID, PriorDeployID: p.PriorDeployID, DatabaseName: p.DatabaseName, Engine: p.Engine,
		BackupID: p.BackupHistoryID, Status: p.Status, Error: p.Error, SizeBytes: p.SizeBytes, CreatedAt: p.CreatedAt,
		RestoredDatabase: p.RestoredDatabase, RestoreStatus: p.RestoreStatus, RestoreError: p.RestoreError,
	}
}

func (rt *Router) refusePredeployUnavailable(w http.ResponseWriter) bool {
	if rt.predeploySnapshots != nil {
		return false
	}
	writeError(w, http.StatusNotImplemented, "pre-deploy snapshots are not available on this control plane")
	return true
}

// handleGetPredeploySnapshotSettings handles GET /api/v1/apps/{name}/predeploy-snapshot.
func (rt *Router) handleGetPredeploySnapshotSettings(w http.ResponseWriter, r *http.Request) {
	if rt.refusePredeployUnavailable(w) {
		return
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get predeploy snapshot settings: load app failed", err, slog.String("name", name))
		return
	}
	s, err := rt.predeploySnapshots.GetPredeploySnapshotSettings(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get predeploy snapshot settings failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, predeploySnapshotSettingsResource{Mode: s.Mode, TargetID: s.TargetID})
}

// handlePutPredeploySnapshotSettings handles PUT /api/v1/apps/{name}/predeploy-snapshot.
func (rt *Router) handlePutPredeploySnapshotSettings(w http.ResponseWriter, r *http.Request) {
	if rt.refusePredeployUnavailable(w) {
		return
	}
	name := r.PathValue("name")
	var req predeploySnapshotSettingsResource
	if !decodeReq(w, r, &req) {
		return
	}
	switch req.Mode {
	case store.PredeployModeOff, store.PredeployModeOn, store.PredeployModeRequire:
	default:
		writeError(w, http.StatusBadRequest, "mode must be one of: off, on, require")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set predeploy snapshot settings: load app failed", err, slog.String("name", name))
		return
	}
	if req.TargetID != "" && !rt.loadBackupTarget(w, r, req.TargetID, "api: set predeploy snapshot settings: load backup target failed") {
		return
	}
	if err := rt.predeploySnapshots.SetPredeploySnapshotSettings(r.Context(), name, store.PredeploySnapshotSettings{Mode: req.Mode, TargetID: req.TargetID}); err != nil {
		rt.internalError(w, "api: set predeploy snapshot settings failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// handleListPredeploySnapshots handles GET /api/v1/apps/{name}/predeploy-snapshots.
func (rt *Router) handleListPredeploySnapshots(w http.ResponseWriter, r *http.Request) {
	if rt.refusePredeployUnavailable(w) {
		return
	}
	name := r.PathValue("name")
	rows, err := rt.predeploySnapshots.ListPredeploySnapshots(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list predeploy snapshots failed", err, slog.String("name", name))
		return
	}
	out := make([]predeploySnapshotResource, 0, len(rows))
	for _, p := range rows {
		out = append(out, toPredeploySnapshotResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// appDatabaseNames lists every managed database the app resolves env from, sorted.
func appDatabaseNames(svc store.DesiredService) []string {
	seen := map[string]bool{}
	for _, ref := range svc.DatabaseEnv {
		seen[ref.Database] = true
	}
	if svc.DatabaseAttachment != nil {
		seen[svc.DatabaseAttachment.DatabaseName] = true
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		if n != "" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func randomPredeploySnapshotID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate predeploy snapshot id: %w", err)
	}
	return "pds_" + hex.EncodeToString(buf), nil
}

// snapshotTargetID picks the settings target, or the only backup target when exactly one exists.
func (rt *Router) snapshotTargetID(ctx context.Context, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	targets, err := rt.backupTargets.ListBackupTargets(ctx)
	if err != nil {
		return "", fmt.Errorf("list backup targets: %w", err)
	}
	if len(targets) != 1 {
		return "", errors.New("no backup target chosen: set one with the snapshot settings")
	}
	return targets[0].ID, nil
}

// priorSucceededDeploy is the deploy a with-data rollback returns the code to.
func (rt *Router) priorSucceededDeploy(ctx context.Context, app string) string {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, app)
	if err != nil {
		return ""
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusSucceeded {
			return a.ID
		}
	}
	return ""
}

// snapshotOneDatabase takes and records one snapshot; the returned row is always persisted.
func (rt *Router) snapshotOneDatabase(ctx context.Context, app, prior, targetID, dbName string) store.PredeploySnapshot {
	row := store.PredeploySnapshot{App: app, PriorDeployID: prior, DatabaseName: dbName, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	fail := func(msg string) store.PredeploySnapshot {
		row.Status, row.Error = store.PredeploySnapshotFailed, msg
		return row
	}
	id, err := randomPredeploySnapshotID()
	if err != nil {
		return fail(err.Error())
	}
	row.ID = id
	db, err := rt.databases.GetDesiredDatabase(ctx, dbName)
	if err != nil {
		return fail("load database: " + err.Error())
	}
	row.Engine = db.Engine
	if !storeEngineBackable(db.Engine) {
		return fail("engine " + db.Engine + " keeps no data on disk")
	}
	historyID, err := randomBackupHistoryID()
	if err != nil {
		return fail(err.Error())
	}
	row.BackupHistoryID = historyID
	runCtx, cancel := context.WithTimeout(ctx, predeploySnapshotTimeout())
	defer cancel()
	if err := rt.backupRunner.RunBackup(runCtx, historyID, dbName, db.Engine, databaseContainerName(dbName), targetID); err != nil {
		return fail("backup: " + err.Error())
	}
	h, err := rt.backupHistory.GetBackupHistory(ctx, historyID)
	if err != nil || h.Status != store.BackupStatusSucceeded {
		return fail("backup did not succeed")
	}
	row.SizeBytes = h.SizeBytes
	if h.SizeBytes > predeploySnapshotMaxBytes() {
		rt.deleteSnapshotBackup(ctx, historyID)
		row.BackupHistoryID = ""
		return fail("snapshot exceeds " + envPredeploySnapshotMaxMB)
	}
	row.Status = store.PredeploySnapshotSucceeded
	return row
}

func storeEngineBackable(engine string) bool { return store.EngineSupportsBackups(engine) }

func (rt *Router) deleteSnapshotBackup(ctx context.Context, historyID string) {
	if historyID == "" || rt.backupDeleter == nil {
		return
	}
	if err := rt.backupDeleter.DeleteBackup(ctx, historyID); err != nil {
		rt.logger.Warn("api: delete predeploy snapshot backup failed", slog.String("id", historyID), slog.String("error", err.Error()))
	}
}

// runPredeploySnapshots snapshots the app's databases when its policy asks for it. It reports
// ok=false (after writing the response) only in require mode when a snapshot failed.
func (rt *Router) runPredeploySnapshots(ctx context.Context, w http.ResponseWriter, svc store.DesiredService) (ids []string, ok bool) {
	if rt.predeploySnapshots == nil || rt.backupRunner == nil {
		return nil, true
	}
	settings, err := rt.predeploySnapshots.GetPredeploySnapshotSettings(ctx, svc.Name)
	if err != nil || settings.Mode == store.PredeployModeOff || settings.Mode == "" {
		return nil, true
	}
	dbs := appDatabaseNames(svc)
	if len(dbs) == 0 {
		return nil, true
	}
	targetID, tErr := rt.snapshotTargetID(ctx, settings.TargetID)
	prior := rt.priorSucceededDeploy(ctx, svc.Name)
	var firstErr string
	for _, dbName := range dbs {
		var row store.PredeploySnapshot
		if tErr != nil {
			row = store.PredeploySnapshot{ID: "", App: svc.Name, PriorDeployID: prior, DatabaseName: dbName, Status: store.PredeploySnapshotFailed, Error: tErr.Error(), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
			if id, idErr := randomPredeploySnapshotID(); idErr == nil {
				row.ID = id
			}
		} else {
			row = rt.snapshotOneDatabase(ctx, svc.Name, prior, targetID, dbName)
		}
		if row.ID == "" {
			continue
		}
		if err := rt.predeploySnapshots.SavePredeploySnapshot(ctx, row); err != nil {
			rt.logger.Error("api: record predeploy snapshot failed", slog.String("id", row.ID), slog.String("error", err.Error()))
			continue
		}
		ids = append(ids, row.ID)
		if row.Status == store.PredeploySnapshotFailed {
			rt.logger.Warn("api: predeploy snapshot failed", slog.String("app", svc.Name), slog.String("database", dbName), slog.String("error", row.Error))
			if firstErr == "" {
				firstErr = dbName + ": " + row.Error
			}
		}
	}
	if firstErr != "" && settings.Mode == store.PredeployModeRequire {
		writeError(w, http.StatusConflict, "pre-deploy snapshot failed and this app requires one: "+firstErr)
		return ids, false
	}
	return ids, true
}

// attachPredeploySnapshots links the snapshots to the deploy they preceded and prunes old ones.
func (rt *Router) attachPredeploySnapshots(ctx context.Context, app string, ids []string, deployID string) {
	if rt.predeploySnapshots == nil || len(ids) == 0 {
		return
	}
	if deployID != "" {
		if err := rt.predeploySnapshots.AttachPredeploySnapshotsToDeploy(ctx, ids, deployID); err != nil {
			rt.logger.Warn("api: attach predeploy snapshots failed", slog.String("app", app), slog.String("error", err.Error()))
		}
	}
	rt.prunePredeploySnapshots(ctx, app)
}

// prunePredeploySnapshots keeps the newest snapshot batches per database and deletes the rest.
func (rt *Router) prunePredeploySnapshots(ctx context.Context, app string) {
	rows, err := rt.predeploySnapshots.ListPredeploySnapshots(ctx, app)
	if err != nil {
		return
	}
	keep := predeploySnapshotKeep()
	perDB := map[string]int{}
	for _, p := range rows {
		perDB[p.DatabaseName]++
		if perDB[p.DatabaseName] <= keep {
			continue
		}
		if p.RestoreStatus == store.PredeployRestoreRunning {
			continue
		}
		rt.deleteSnapshotBackup(ctx, p.BackupHistoryID)
		if err := rt.predeploySnapshots.DeletePredeploySnapshot(ctx, p.ID); err != nil {
			rt.logger.Warn("api: prune predeploy snapshot failed", slog.String("id", p.ID), slog.String("error", err.Error()))
		}
	}
}

type rollbackWithDataResource struct {
	DeployID   string                      `json:"deploy_id"`
	RollbackTo string                      `json:"rollback_to"`
	Snapshots  []predeploySnapshotResource `json:"snapshots"`
}

// swapDatabaseLinks points the app's env refs from old database names to their restored copies.
func swapDatabaseLinks(svc store.DesiredService, swaps map[string]string) store.DesiredService {
	if len(svc.DatabaseEnv) > 0 {
		env := make(map[string]store.DatabaseEnvRef, len(svc.DatabaseEnv))
		for k, ref := range svc.DatabaseEnv {
			if n, ok := swaps[ref.Database]; ok {
				ref.Database = n
			}
			env[k] = ref
		}
		svc.DatabaseEnv = env
	}
	if svc.DatabaseAttachment != nil {
		att := *svc.DatabaseAttachment
		if n, ok := swaps[att.DatabaseName]; ok {
			att.DatabaseName = n
		}
		svc.DatabaseAttachment = &att
	}
	return svc
}

func newRestoredDatabaseName(base string) (string, error) {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate restored database name: %w", err)
	}
	return base + "-rb-" + hex.EncodeToString(buf), nil
}

// handleRollbackWithData handles POST /api/v1/apps/{name}/deploys/{deployId}/rollback-with-data:
// restores the snapshots taken before deployId as new databases, then swaps the app's links to them
// and redeploys the previous release. The old databases are never touched.
func (rt *Router) handleRollbackWithData(w http.ResponseWriter, r *http.Request) {
	if rt.refusePredeployUnavailable(w) {
		return
	}
	if rt.cloneRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}
	name, deployID := r.PathValue("name"), r.PathValue("deployId")
	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: rollback with data: load app failed", err, slog.String("name", name))
		return
	}
	rows, err := rt.predeploySnapshots.ListPredeploySnapshotsForDeploy(r.Context(), name, deployID)
	if err != nil {
		rt.internalError(w, "api: rollback with data: list snapshots failed", err, slog.String("name", name))
		return
	}
	var usable []store.PredeploySnapshot
	for _, p := range rows {
		if p.Status == store.PredeploySnapshotSucceeded && p.BackupHistoryID != "" && p.RestoreStatus != store.PredeployRestoreRunning {
			usable = append(usable, p)
		}
	}
	if len(usable) == 0 {
		writeError(w, http.StatusConflict, "no usable pre-deploy snapshot was recorded for this deploy")
		return
	}
	priorID := usable[0].PriorDeployID
	target, ok := rt.findAttempt(r.Context(), name, priorID)
	if !ok {
		writeError(w, http.StatusConflict, "the release before this deploy is no longer available to roll back to")
		return
	}
	if env, err := rt.environments.GetEnvironment(r.Context(), existing.EnvironmentID); existing.EnvironmentID != "" && err == nil && env.Protected {
		writeError(w, http.StatusConflict, "this app is in a protected environment: use the plain rollback and restore the data by hand")
		return
	}
	if _, ok := rt.freezeGate(w, r, name, freezeOverride{}); !ok {
		return
	}
	pin := rt.rollbackPin(r.Context(), target)
	ref, status, msg := rt.rollbackImageRef(r.Context(), target, pin)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	if rt.hasRunningDeployAttempt(r.Context(), name) {
		writeError(w, http.StatusConflict, "a deploy for this app is already running")
		return
	}

	type job struct {
		snap    store.PredeploySnapshot
		newName string
		cloneID string
		engine  string
	}
	jobs := make([]job, 0, len(usable))
	for _, p := range usable {
		newName, err := newRestoredDatabaseName(p.DatabaseName)
		if err != nil {
			rt.internalError(w, "api: rollback with data: name failed", err)
			return
		}
		src, err := rt.databases.GetDesiredDatabase(r.Context(), p.DatabaseName)
		if err != nil {
			writeError(w, http.StatusConflict, "database "+p.DatabaseName+" no longer exists")
			return
		}
		if !rt.createDesiredDatabase(w, r, databaseResource{Name: newName, Engine: src.Engine, Version: src.Version, ProjectID: src.ProjectID}) {
			return
		}
		cloneID, err := randomCloneRestoreID()
		if err != nil {
			rt.internalError(w, "api: rollback with data: id failed", err)
			return
		}
		if err := rt.predeploySnapshots.SetPredeploySnapshotRestore(r.Context(), p.ID, newName, store.PredeployRestoreRunning, ""); err != nil {
			rt.internalError(w, "api: rollback with data: record restore failed", err)
			return
		}
		p.RestoredDatabase, p.RestoreStatus = newName, store.PredeployRestoreRunning
		jobs = append(jobs, job{snap: p, newName: newName, cloneID: cloneID, engine: src.Engine})
	}

	// The restore outlives the request, so it must not use r.Context().
	go rt.finishRollbackWithData( //nolint:gosec // detached on purpose
		name, target, ref, pin, func(ctx context.Context) map[string]string {
			swaps := map[string]string{}
			for _, j := range jobs {
				err := rt.cloneRestoreRunner.RunCloneRestore(ctx, j.cloneID, j.snap.DatabaseName, j.newName, j.snap.BackupHistoryID, j.engine, databaseContainerName(j.newName), databaseControllerName(j.newName))
				if err != nil {
					rt.logger.Error("api: rollback with data: restore failed", slog.String("app", name), slog.String("database", j.snap.DatabaseName), slog.String("error", err.Error()))
					_ = rt.predeploySnapshots.SetPredeploySnapshotRestore(ctx, j.snap.ID, j.newName, store.PredeployRestoreFailed, err.Error())
					return nil
				}
				_ = rt.predeploySnapshots.SetPredeploySnapshotRestore(ctx, j.snap.ID, j.newName, store.PredeployRestoreSucceeded, "")
				swaps[j.snap.DatabaseName] = j.newName
			}
			return swaps
		})

	out := rollbackWithDataResource{DeployID: deployID, RollbackTo: priorID}
	for _, j := range jobs {
		out.Snapshots = append(out.Snapshots, toPredeploySnapshotResource(j.snap))
	}
	writeJSON(w, http.StatusAccepted, out)
}

// findAttempt returns the app's deploy attempt with this id.
func (rt *Router) findAttempt(ctx context.Context, app, id string) (store.DeployAttempt, bool) {
	if id == "" {
		return store.DeployAttempt{}, false
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, app)
	if err != nil {
		return store.DeployAttempt{}, false
	}
	for _, a := range attempts {
		if a.ID == id {
			return a, true
		}
	}
	return store.DeployAttempt{}, false
}

// finishRollbackWithData runs detached from the request: restores, swaps links, then redeploys.
func (rt *Router) finishRollbackWithData(app string, target store.DeployAttempt, ref string, pin *store.RollbackPin, restore func(ctx context.Context) map[string]string) {
	ctx := context.Background()
	swaps := restore(ctx)
	if swaps == nil {
		return
	}
	current, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		rt.logger.Error("api: rollback with data: reload app failed", slog.String("app", app), slog.String("error", err.Error()))
		return
	}
	restored := swapDatabaseLinks(deploy.RestoreSnapshot(*current, target.Snapshot), swaps)
	if deploy.LocalOnlyRef(pin, ref) {
		restored.PullPolicy = ""
	}
	if _, err := rt.executeConfirmedDeploy(ctx, restored, ref, confirmedDeployOptions{reason: "RollbackTo: " + target.ID + " with data"}); err != nil {
		rt.logger.Error("api: rollback with data: redeploy failed", slog.String("app", app), slog.String("error", err.Error()))
		return
	}
	rt.recordOperatorRollback(ctx, app)
}

func predeployStoreOf(s Store) PredeploySnapshotStore {
	p, _ := s.(PredeploySnapshotStore)
	return p
}

func (rt *Router) registerPredeploySnapshotRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/predeploy-snapshot", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPredeploySnapshotSettings))
	mux.HandleFunc("PUT /api/v1/apps/{name}/predeploy-snapshot", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutPredeploySnapshotSettings))
	mux.HandleFunc("GET /api/v1/apps/{name}/predeploy-snapshots", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPredeploySnapshots))
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys/{deployId}/rollback-with-data", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.idempotent(rt.handleRollbackWithData)))
}
