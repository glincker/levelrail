package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// BackupProtection is the surface the backup health, drill and restore-to
// routes need from internal/backup.Protection.
type BackupProtection interface {
	Health(ctx context.Context) ([]backup.ResourceHealth, error)
	Encryption() backup.EncryptionStatus
	StartDrill(ctx context.Context, backupID string) (string, error)
	RefreshProtection(ctx context.Context, targetIDs []string) ([]store.BackupTargetProtection, error)
	VolumeExists(ctx context.Context, rtm docker.Runtime, name string) (exists, known bool, err error)
	RestoreVolumeTo(ctx context.Context, rtm docker.Runtime, historyID, sourceService, sourceVolume, newVolume, backupID string) error
}

// BackupProtectionStore is the store surface the same routes read and write.
type BackupProtectionStore interface {
	ListBackupDrills(ctx context.Context, serviceName, volumeName, databaseName string, limit int) ([]store.BackupDrill, error)
	GetBackupDrill(ctx context.Context, id string) (store.BackupDrill, error)
	GetVolumeBackupPolicy(ctx context.Context, serviceName, volumeName string) (store.VolumeBackupPolicy, error)
	SetVolumeBackupPolicy(ctx context.Context, p store.VolumeBackupPolicy) error
	ListBackupTargetProtection(ctx context.Context) ([]store.BackupTargetProtection, error)
}

type backupSummaryResource struct {
	ID        string `json:"id"`
	At        string `json:"at"`
	Status    string `json:"status"`
	SizeBytes int64  `json:"size_bytes"`
	Codec     string `json:"codec,omitempty"`
	Error     string `json:"error,omitempty"`
}

type drillSummaryResource struct {
	ID      string `json:"id"`
	At      string `json:"at"`
	Status  string `json:"status"`
	Stage   string `json:"stage,omitempty"`
	Error   string `json:"error,omitempty"`
	Trigger string `json:"trigger,omitempty"`
	Files   int64  `json:"files,omitempty"`
}

type resourceHealthResource struct {
	Kind                string                 `json:"kind"`
	AppName             string                 `json:"app_name,omitempty"`
	ResourceName        string                 `json:"resource_name"`
	TargetID            string                 `json:"target_id,omitempty"`
	Schedule            string                 `json:"schedule,omitempty"`
	NextRun             string                 `json:"next_run,omitempty"`
	BackupCount         int                    `json:"backup_count"`
	TotalBytes          int64                  `json:"total_bytes"`
	LastAttempt         *backupSummaryResource `json:"last_attempt,omitempty"`
	LastBackup          *backupSummaryResource `json:"last_backup,omitempty"`
	LastVerifiedRestore *drillSummaryResource  `json:"last_verified_restore,omitempty"`
	LastDrill           *drillSummaryResource  `json:"last_drill,omitempty"`
	Encrypted           bool                   `json:"encrypted"`
	State               string                 `json:"state"`
	StateReason         string                 `json:"state_reason,omitempty"`
	ProtectionLevel     string                 `json:"protection_level,omitempty"`
	Warning             string                 `json:"warning,omitempty"`
}

type targetProtectionResource struct {
	TargetID   string `json:"target_id"`
	Level      string `json:"level"`
	ObjectLock bool   `json:"object_lock"`
	LockMode   string `json:"lock_mode,omitempty"`
	Versioning string `json:"versioning,omitempty"`
	CanDelete  bool   `json:"can_delete"`
	Warning    string `json:"warning,omitempty"`
	ProbeError string `json:"probe_error,omitempty"`
	CheckedAt  string `json:"checked_at"`
}

type backupHealthResponse struct {
	Resources  []resourceHealthResource   `json:"resources"`
	Targets    []targetProtectionResource `json:"targets"`
	Encryption struct {
		Enabled   bool   `json:"enabled"`
		Recipient string `json:"recipient,omitempty"`
		Codec     string `json:"codec"`
	} `json:"encryption"`
}

func toBackupSummary(s *backup.HealthBackup) *backupSummaryResource {
	if s == nil {
		return nil
	}
	return &backupSummaryResource{ID: s.ID, At: s.At, Status: s.Status, SizeBytes: s.SizeBytes, Codec: s.Codec, Error: s.Error}
}

func toDrillSummary(s *backup.DrillSummary) *drillSummaryResource {
	if s == nil {
		return nil
	}
	return &drillSummaryResource{ID: s.ID, At: s.At, Status: s.Status, Stage: s.Stage, Error: s.Error, Trigger: s.Trigger, Files: s.Files}
}

func toTargetProtection(p store.BackupTargetProtection) targetProtectionResource {
	bp := backup.BucketProtection{ObjectLock: p.ObjectLock, LockMode: p.LockMode, Versioning: p.Versioning, CanDelete: p.CanDelete}
	out := targetProtectionResource{
		TargetID: p.TargetID, Level: bp.Level(), ObjectLock: p.ObjectLock, LockMode: p.LockMode,
		Versioning: p.Versioning, CanDelete: p.CanDelete, ProbeError: p.ProbeError, CheckedAt: p.CheckedAt,
	}
	if p.ProbeError == "" {
		out.Warning = bp.Warning()
	}
	return out
}

// handleBackupHealth handles GET /api/v1/backups/health: one row per backed
// up or scheduled database and app volume, with last backup, last verified
// restore, size and next run.
func (rt *Router) handleBackupHealth(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "backup health is not configured on this control plane")
		return
	}
	rows, err := rt.backupProtection.Health(r.Context())
	if err != nil {
		rt.internalError(w, "api: backup health failed", err)
		return
	}
	prot, err := rt.backupProtectionStore.ListBackupTargetProtection(r.Context())
	if err != nil {
		rt.internalError(w, "api: list backup target protection failed", err)
		return
	}
	resp := backupHealthResponse{Resources: make([]resourceHealthResource, 0, len(rows)), Targets: make([]targetProtectionResource, 0, len(prot))}
	for _, h := range rows {
		resp.Resources = append(resp.Resources, resourceHealthResource{
			Kind: h.Kind, AppName: h.AppName, ResourceName: h.ResourceName, TargetID: h.TargetID, Schedule: h.Schedule,
			NextRun: h.NextRun, BackupCount: h.BackupCount, TotalBytes: h.TotalBytes,
			LastAttempt: toBackupSummary(h.LastAttempt), LastBackup: toBackupSummary(h.LastBackup),
			LastVerifiedRestore: toDrillSummary(h.LastVerifiedRestore), LastDrill: toDrillSummary(h.LastDrill),
			Encrypted: h.Encrypted, State: h.State, StateReason: h.StateReason, ProtectionLevel: h.ProtectionLevel, Warning: h.Warning,
		})
	}
	for _, p := range prot {
		resp.Targets = append(resp.Targets, toTargetProtection(p))
	}
	enc := rt.backupProtection.Encryption()
	resp.Encryption.Enabled, resp.Encryption.Recipient, resp.Encryption.Codec = enc.Enabled, enc.Recipient, enc.Codec
	writeJSON(w, http.StatusOK, resp)
}

type drillResource struct {
	ID           string `json:"id"`
	BackupID     string `json:"backup_id"`
	ResourceKind string `json:"resource_kind"`
	DatabaseName string `json:"database_name,omitempty"`
	ServiceName  string `json:"service_name,omitempty"`
	VolumeName   string `json:"volume_name,omitempty"`
	Trigger      string `json:"trigger"`
	Status       string `json:"status"`
	Stage        string `json:"stage"`
	ObjectOK     bool   `json:"object_ok"`
	ChecksumOK   bool   `json:"checksum_ok"`
	RestoreOK    bool   `json:"restore_ok"`
	ContentOK    bool   `json:"content_ok"`
	Files        int64  `json:"files"`
	Bytes        int64  `json:"bytes"`
	DurationMS   int64  `json:"duration_ms"`
	Error        string `json:"error,omitempty"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

func toDrillResource(d store.BackupDrill) drillResource {
	return drillResource{
		ID: d.ID, BackupID: d.BackupHistoryID, ResourceKind: d.ResourceKind, DatabaseName: d.DatabaseName,
		ServiceName: d.ServiceName, VolumeName: d.VolumeName, Trigger: d.Trigger, Status: d.Status, Stage: d.Stage,
		ObjectOK: d.ObjectOK, ChecksumOK: d.ChecksumOK, RestoreOK: d.RestoreOK, ContentOK: d.ContentOK,
		Files: d.Files, Bytes: d.Bytes, DurationMS: d.DurationMS, Error: d.Error, StartedAt: d.StartedAt, FinishedAt: d.FinishedAt,
	}
}

// handleListBackupDrills handles GET /api/v1/backups/drills, optionally
// narrowed by ?service=&volume= or ?database= and bounded by ?limit=.
func (rt *Router) handleListBackupDrills(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtectionStore == nil {
		writeError(w, http.StatusNotImplemented, "restore drills are not configured on this control plane")
		return
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	drills, err := rt.backupProtectionStore.ListBackupDrills(r.Context(), q.Get("service"), q.Get("volume"), q.Get("database"), limit)
	if err != nil {
		rt.internalError(w, "api: list backup drills failed", err)
		return
	}
	out := make([]drillResource, 0, len(drills))
	for _, d := range drills {
		out = append(out, toDrillResource(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetBackupDrill handles GET /api/v1/backups/drills/{id}.
func (rt *Router) handleGetBackupDrill(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtectionStore == nil {
		writeError(w, http.StatusNotImplemented, "restore drills are not configured on this control plane")
		return
	}
	d, err := rt.backupProtectionStore.GetBackupDrill(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "drill not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get backup drill failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toDrillResource(d))
}

type startDrillRequest struct {
	BackupID string `json:"backup_id"`
}

// handleStartBackupDrill handles POST /api/v1/backups/drills: restores the
// named backup into a scratch resource, validates and destroys it. Returns
// 202 with the drill ID to poll.
func (rt *Router) handleStartBackupDrill(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "restore drills are not configured on this control plane")
		return
	}
	var req startDrillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.BackupID) == "" {
		writeError(w, http.StatusBadRequest, "backup_id is required")
		return
	}
	h, err := rt.backupHistory.GetBackupHistory(r.Context(), req.BackupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: start drill: load backup failed", err, slog.String("backup_id", req.BackupID))
		return
	}
	if h.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be drilled", req.BackupID, h.Status))
		return
	}
	id, err := rt.backupProtection.StartDrill(r.Context(), req.BackupID)
	if err != nil {
		rt.internalError(w, "api: start drill failed", err, slog.String("backup_id", req.BackupID))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "backup_id": req.BackupID})
}

type refreshProtectionRequest struct {
	TargetIDs []string `json:"target_ids"`
}

// handleRefreshBackupProtection handles POST /api/v1/backups/protection/
// refresh: re-probes bucket object lock and versioning for the named targets
// (every target when none are named).
func (rt *Router) handleRefreshBackupProtection(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "bucket protection probing is not configured on this control plane")
		return
	}
	var req refreshProtectionRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	ids := req.TargetIDs
	if len(ids) == 0 {
		targets, err := rt.backupTargets.ListBackupTargets(r.Context())
		if err != nil {
			rt.internalError(w, "api: refresh protection: list targets failed", err)
			return
		}
		for _, t := range targets {
			ids = append(ids, t.ID)
		}
	}
	res, err := rt.backupProtection.RefreshProtection(r.Context(), ids)
	if err != nil {
		rt.internalError(w, "api: refresh bucket protection failed", err)
		return
	}
	out := make([]targetProtectionResource, 0, len(res))
	for _, p := range res {
		out = append(out, toTargetProtection(p))
	}
	writeJSON(w, http.StatusOK, out)
}

type volumeBackupPolicyResource struct {
	ServiceName   string `json:"service_name"`
	VolumeName    string `json:"volume_name"`
	RetainDaily   int    `json:"retain_daily"`
	RetainWeekly  int    `json:"retain_weekly"`
	RetainMonthly int    `json:"retain_monthly"`
	PreHook       string `json:"pre_hook"`
	PostHook      string `json:"post_hook"`
	Quiesce       string `json:"quiesce"`
}

// handleGetVolumeBackupPolicy handles
// GET /api/v1/apps/{name}/volumes/{volume}/backup-policy.
func (rt *Router) handleGetVolumeBackupPolicy(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtectionStore == nil {
		writeError(w, http.StatusNotImplemented, "backup policies are not configured on this control plane")
		return
	}
	svc, vol := r.PathValue("name"), r.PathValue("volume")
	if _, ok := rt.loadServiceVolume(w, r, svc, vol, "api: get volume backup policy: load service failed"); !ok {
		return
	}
	p, err := rt.backupProtectionStore.GetVolumeBackupPolicy(r.Context(), svc, vol)
	if err != nil && !errors.Is(err, store.ErrVolumeBackupPolicyNotFound) {
		rt.internalError(w, "api: get volume backup policy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, volumeBackupPolicyResource{
		ServiceName: svc, VolumeName: vol, RetainDaily: p.RetainDaily, RetainWeekly: p.RetainWeekly,
		RetainMonthly: p.RetainMonthly, PreHook: p.PreHook, PostHook: p.PostHook, Quiesce: p.Quiesce,
	})
}

// maxHookLength bounds a pre or post backup hook command.
const maxHookLength = 2048

// handleSetVolumeBackupPolicy handles
// PUT /api/v1/apps/{name}/volumes/{volume}/backup-policy.
func (rt *Router) handleSetVolumeBackupPolicy(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtectionStore == nil {
		writeError(w, http.StatusNotImplemented, "backup policies are not configured on this control plane")
		return
	}
	svc, vol := r.PathValue("name"), r.PathValue("volume")
	if _, ok := rt.loadServiceVolume(w, r, svc, vol, "api: set volume backup policy: load service failed"); !ok {
		return
	}
	var req volumeBackupPolicyResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch {
	case req.RetainDaily < 0 || req.RetainWeekly < 0 || req.RetainMonthly < 0:
		writeError(w, http.StatusBadRequest, "retention counts must not be negative")
		return
	case req.Quiesce != "" && req.Quiesce != store.VolumeQuiescePause:
		writeError(w, http.StatusBadRequest, `quiesce must be "" or "pause"`)
		return
	case len(req.PreHook) > maxHookLength || len(req.PostHook) > maxHookLength:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("hooks are limited to %d characters", maxHookLength))
		return
	}
	p := store.VolumeBackupPolicy{
		ServiceName: svc, VolumeName: vol, RetainDaily: req.RetainDaily, RetainWeekly: req.RetainWeekly,
		RetainMonthly: req.RetainMonthly, PreHook: strings.TrimSpace(req.PreHook), PostHook: strings.TrimSpace(req.PostHook), Quiesce: req.Quiesce,
	}
	if err := rt.backupProtectionStore.SetVolumeBackupPolicy(r.Context(), p); err != nil {
		rt.internalError(w, "api: set volume backup policy failed", err)
		return
	}
	req.ServiceName, req.VolumeName, req.PreHook, req.PostHook = svc, vol, p.PreHook, p.PostHook
	writeJSON(w, http.StatusOK, req)
}

type volumeRestoreToRequest struct {
	BackupID      string `json:"backup_id"`
	NewVolumeName string `json:"new_volume_name,omitempty"`
	// TargetApp names the app the new volume is for: the volume gets the
	// name that app's own declared volume of the same logical name resolves
	// to, so deploying it picks the restored data up.
	TargetApp string `json:"target_app,omitempty"`
	// NodeID restores onto that node instead of the source app's node.
	NodeID string `json:"node_id,omitempty"`
}

// handleVolumeRestoreTo handles
// POST /api/v1/apps/{name}/volumes/{volume}/restore-to: restores a volume
// backup into a new volume, optionally named for another app and optionally
// on another node. It never writes to an existing volume.
func (rt *Router) handleVolumeRestoreTo(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil || rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "restore to another node is not configured on this control plane")
		return
	}
	svcName, volName := r.PathValue("name"), r.PathValue("volume")
	if _, ok := rt.loadServiceVolume(w, r, svcName, volName, "api: volume restore-to: load service failed"); !ok {
		return
	}
	var req volumeRestoreToRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BackupID == "" {
		writeError(w, http.StatusBadRequest, "backup_id is required")
		return
	}
	bh, err := rt.backupHistory.GetBackupHistory(r.Context(), req.BackupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: volume restore-to: load backup failed", err, slog.String("backup_id", req.BackupID))
		return
	}
	if bh.ServiceName != svcName || bh.VolumeName != volName {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was not taken from %s/%s", req.BackupID, svcName, volName))
		return
	}
	if bh.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be restored from", req.BackupID, bh.Status))
		return
	}

	nodeID := req.NodeID
	if nodeID == "" {
		src, err := rt.apps.GetDesiredService(r.Context(), svcName)
		if err != nil {
			rt.internalError(w, "api: volume restore-to: load app failed", err, slog.String("name", svcName))
			return
		}
		nodeID = src.NodeID
	}
	nodeRuntime, err := rt.execRuntime(nodeID)
	if err != nil {
		rt.logger.Error("api: volume restore-to: resolve node runtime failed", slog.String("error", err.Error()), slog.String("node_id", nodeID))
		writeError(w, http.StatusBadGateway, "the chosen node is not currently reachable")
		return
	}

	newVolume, explicit, ok := rt.chooseRestoreVolumeName(w, req, svcName, volName)
	if !ok {
		return
	}
	exists, known, err := rt.backupProtection.VolumeExists(r.Context(), nodeRuntime, newVolume)
	if err != nil {
		rt.internalError(w, "api: volume restore-to: check volume failed", err, slog.String("volume", newVolume))
		return
	}
	if exists {
		writeError(w, http.StatusConflict, fmt.Sprintf("volume %q already exists on that node: restores never overwrite, choose another name or remove it first", newVolume))
		return
	}
	if !known && explicit {
		writeError(w, http.StatusConflict, "that node cannot confirm the volume name is unused: leave new_volume_name empty to get a generated name")
		return
	}

	historyID, err := randomVolumeCloneRestoreID()
	if err != nil {
		rt.internalError(w, "api: volume restore-to: generate id failed", err)
		return
	}
	go func() { //nolint:gosec // deliberately not r.Context(): cancelled the moment this handler returns
		if err := rt.backupProtection.RestoreVolumeTo(context.Background(), nodeRuntime, historyID, svcName, volName, newVolume, req.BackupID); err != nil {
			rt.logger.Error("api: volume restore-to failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("service", svcName), slog.String("volume", volName))
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"id": historyID, "new_volume_name": newVolume, "node_id": nodeID})
}

// chooseRestoreVolumeName resolves the new volume's name from the request,
// writing the error response itself on a bad request.
func (rt *Router) chooseRestoreVolumeName(w http.ResponseWriter, req volumeRestoreToRequest, svcName, volName string) (name string, explicit, ok bool) {
	switch {
	case strings.TrimSpace(req.NewVolumeName) != "":
		name = strings.TrimSpace(req.NewVolumeName)
		explicit = true
	case strings.TrimSpace(req.TargetApp) != "":
		name = "app-" + strings.TrimSpace(req.TargetApp) + "-" + volName
		explicit = true
	default:
		generated, err := defaultVolumeCloneName(svcName, volName)
		if err != nil {
			rt.internalError(w, "api: volume restore-to: generate name failed", err)
			return "", false, false
		}
		name = generated
	}
	if err := backup.ValidateVolumeName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false, false
	}
	return name, explicit, true
}
