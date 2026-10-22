package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envBackupDefaultSchedule   = "APP_BACKUP_DEFAULT_SCHEDULE"
	envBackupDefaultRetain     = "APP_BACKUP_DEFAULT_RETAIN"
	envBackupDefaultRetainDays = "APP_BACKUP_DEFAULT_RETAIN_DAYS"

	fallbackBackupSchedule = "0 3 * * *"
	fallbackBackupRetain   = 7
	fallbackBackupDays     = 30
)

// backupDefaults is the schedule and retention suggested to a resource that
// has none. It is a recommendation: nothing applies it without a request.
type backupDefaults struct {
	Schedule   string `json:"schedule"`
	Retain     int    `json:"retain"`
	RetainDays int    `json:"retain_days"`
}

func currentBackupDefaults() backupDefaults {
	d := backupDefaults{Schedule: fallbackBackupSchedule, Retain: fallbackBackupRetain, RetainDays: fallbackBackupDays}
	if s := strings.TrimSpace(os.Getenv(envBackupDefaultSchedule)); s != "" {
		d.Schedule = s
	}
	d.Retain = envInt(envBackupDefaultRetain, d.Retain)
	d.RetainDays = envInt(envBackupDefaultRetainDays, d.RetainDays)
	return d
}

type coverageRowResource struct {
	Kind            string `json:"kind"`
	AppName         string `json:"app_name,omitempty"`
	Name            string `json:"name"`
	Scratch         bool   `json:"scratch,omitempty"`
	State           string `json:"state"`
	Reason          string `json:"reason,omitempty"`
	PrimaryAction   string `json:"primary_action"`
	Schedule        string `json:"schedule,omitempty"`
	TargetID        string `json:"target_id,omitempty"`
	NextRun         string `json:"next_run,omitempty"`
	LastBackupAt    string `json:"last_backup_at,omitempty"`
	LastBackupID    string `json:"last_backup_id,omitempty"`
	LastVerifiedAt  string `json:"last_verified_at,omitempty"`
	TotalBytes      int64  `json:"total_bytes"`
	BackupCount     int    `json:"backup_count"`
	Encrypted       bool   `json:"encrypted"`
	ProtectionLevel string `json:"protection_level,omitempty"`
}

type coverageSummaryResource struct {
	Total               int    `json:"total"`
	Protected           int    `json:"protected"`
	Verified            int    `json:"verified"`
	Unverified          int    `json:"unverified"`
	NeverRan            int    `json:"never_ran"`
	Failing             int    `json:"failing"`
	Unprotected         int    `json:"unprotected"`
	Scratch             int    `json:"scratch"`
	StorageBytes        int64  `json:"storage_bytes"`
	LastVerifiedRestore string `json:"last_verified_restore,omitempty"`
	NextRun             string `json:"next_run,omitempty"`
}

type coverageTargetResource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type coverageEncryptionResource struct {
	Enabled   bool   `json:"enabled"`
	Recipient string `json:"recipient,omitempty"`
	Codec     string `json:"codec,omitempty"`
	// KeyStoredOffBox is true once the operator acknowledged storing the
	// private key off this server for the current public key.
	KeyStoredOffBox bool   `json:"key_stored_off_box"`
	AcknowledgedAt  string `json:"acknowledged_at,omitempty"`
}

type backupCoverageResponse struct {
	Summary    coverageSummaryResource    `json:"summary"`
	Resources  []coverageRowResource      `json:"resources"`
	Targets    []coverageTargetResource   `json:"targets"`
	Defaults   backupDefaults             `json:"defaults"`
	Encryption coverageEncryptionResource `json:"encryption"`
}

// backupKeyAckStore is satisfied by *store.DB. It is asserted rather than
// added to BackupProtectionStore so existing fakes keep compiling.
type backupKeyAckStore interface {
	GetBackupKeyAck(ctx context.Context) (store.BackupKeyAck, bool, error)
	SetBackupKeyAck(ctx context.Context, recipient, actor string, now time.Time) error
}

func (rt *Router) encryptionResource(ctx context.Context) coverageEncryptionResource {
	enc := rt.backupProtection.Encryption()
	out := coverageEncryptionResource{Enabled: enc.Enabled, Recipient: enc.Recipient, Codec: enc.Codec}
	acks, ok := rt.backupProtectionStore.(backupKeyAckStore)
	if !ok || !enc.Enabled {
		return out
	}
	ack, found, err := acks.GetBackupKeyAck(ctx)
	if err != nil {
		rt.logger.Warn("api: backup coverage: read key acknowledgement failed", slog.String("error", err.Error()))
		return out
	}
	if found && ack.Recipient == enc.Recipient {
		out.KeyStoredOffBox = true
		out.AcknowledgedAt = ack.AcknowledgedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// handleAckBackupEncryptionKey handles POST
// /api/v1/backups/encryption-key/ack: the operator states the private key is
// stored off this server. It records the claim, it cannot verify it.
func (rt *Router) handleAckBackupEncryptionKey(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "backup encryption is not configured on this control plane")
		return
	}
	enc := rt.backupProtection.Encryption()
	acks, ok := rt.backupProtectionStore.(backupKeyAckStore)
	if !enc.Enabled || !ok {
		writeError(w, http.StatusConflict, "backups are not encrypted on this control plane, there is no key to acknowledge")
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if err := acks.SetBackupKeyAck(r.Context(), enc.Recipient, actor, time.Now()); err != nil {
		rt.internalError(w, "api: acknowledge backup key failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.encryptionResource(r.Context()))
}

// coverageResources lists every database and app volume, flagging preview and canary data as scratch.
func (rt *Router) coverageResources(ctx context.Context, health []backup.ResourceHealth) ([]backup.CoverageResource, error) {
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	dbs = rt.withoutPendingDatabases(ctx, dbs)
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	scratchApps, scratchDBs, err := rt.scratchResourceNames(ctx)
	if err != nil {
		return nil, err
	}
	volSchedule := make(map[[2]string]backup.ResourceHealth)
	for _, h := range health {
		if h.Kind == store.BackupResourceKindVolume {
			volSchedule[[2]string{h.AppName, h.ResourceName}] = h
		}
	}

	out := make([]backup.CoverageResource, 0, len(dbs)+len(services))
	for _, d := range dbs {
		out = append(out, backup.CoverageResource{
			Kind: store.BackupResourceKindDatabase, Name: d.Name,
			Schedule: d.BackupSchedule, TargetID: d.BackupTargetID, Scratch: scratchDBs[d.Name],
		})
	}
	for _, svc := range services {
		scratch := scratchApps[svc.Name] || store.IsCanaryService(svc.Name)
		for _, v := range toAppVolumeResources(svc) {
			res := backup.CoverageResource{Kind: store.BackupResourceKindVolume, AppName: svc.Name, Name: v.Name, Scratch: scratch}
			if h, ok := volSchedule[[2]string{svc.Name, v.Name}]; ok {
				res.Schedule, res.TargetID = h.Schedule, h.TargetID
			}
			out = append(out, res)
		}
	}
	return out, nil
}

func (rt *Router) scratchResourceNames(ctx context.Context) (apps, databases map[string]bool, err error) {
	apps, databases = map[string]bool{}, map[string]bool{}
	envs, err := rt.previewEnvironments.ListPreviewEnvironments(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list preview environments: %w", err)
	}
	for _, p := range envs {
		apps[p.PreviewAppID] = true
		eph, err := rt.previewEnvironments.ListPreviewEphemeralDatabasesByPreview(ctx, p.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list preview databases of %q: %w", p.ID, err)
		}
		for _, e := range eph {
			databases[e.DatabaseName] = true
		}
	}
	return apps, databases, nil
}

func (rt *Router) buildCoverage(ctx context.Context) ([]backup.CoverageRow, backup.CoverageSummary, error) {
	health, err := rt.backupProtection.Health(ctx)
	if err != nil {
		return nil, backup.CoverageSummary{}, fmt.Errorf("backup health: %w", err)
	}
	res, err := rt.coverageResources(ctx, health)
	if err != nil {
		return nil, backup.CoverageSummary{}, err
	}
	rows, sum := backup.DeriveCoverage(res, health)
	return rows, sum, nil
}

// handleBackupCoverage handles GET /api/v1/backups/coverage: every database
// and app volume with whether it is protected, verified or not backed up.
func (rt *Router) handleBackupCoverage(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "backup coverage is not configured on this control plane")
		return
	}
	rows, sum, err := rt.buildCoverage(r.Context())
	if err != nil {
		rt.internalError(w, "api: backup coverage failed", err)
		return
	}
	targets, err := rt.backupTargets.ListBackupTargets(r.Context())
	if err != nil {
		rt.internalError(w, "api: backup coverage: list targets failed", err)
		return
	}
	resp := backupCoverageResponse{
		Summary: coverageSummaryResource{
			Total: sum.Total, Protected: sum.Protected, Verified: sum.Verified, Unverified: sum.Unverified,
			NeverRan: sum.NeverRan, Failing: sum.Failing, Unprotected: sum.Unprotected, Scratch: sum.Scratch,
			StorageBytes: sum.StorageBytes, LastVerifiedRestore: sum.LastVerifiedRestore, NextRun: sum.NextRun,
		},
		Resources:  make([]coverageRowResource, 0, len(rows)),
		Targets:    make([]coverageTargetResource, 0, len(targets)),
		Defaults:   currentBackupDefaults(),
		Encryption: rt.encryptionResource(r.Context()),
	}
	for _, c := range rows {
		resp.Resources = append(resp.Resources, coverageRowResource{
			Kind: c.Kind, AppName: c.AppName, Name: c.Name, Scratch: c.Scratch, State: c.State, Reason: c.Reason,
			PrimaryAction: c.PrimaryAction, Schedule: c.Schedule, TargetID: c.TargetID, NextRun: c.NextRun,
			LastBackupAt: c.LastBackupAt, LastBackupID: c.LastBackupID, LastVerifiedAt: c.LastVerifiedAt,
			TotalBytes: c.TotalBytes, BackupCount: c.BackupCount, Encrypted: c.Encrypted, ProtectionLevel: c.ProtectionLevel,
		})
	}
	for _, t := range targets {
		resp.Targets = append(resp.Targets, coverageTargetResource{ID: t.ID, Name: t.Name, Provider: t.Provider})
	}
	writeJSON(w, http.StatusOK, resp)
}

type protectAllRequest struct {
	TargetID       string `json:"target_id"`
	Schedule       string `json:"schedule,omitempty"`
	Retain         *int   `json:"retain,omitempty"`
	RetainDays     *int   `json:"retain_days,omitempty"`
	IncludeScratch bool   `json:"include_scratch,omitempty"`
	// Only narrows the plan to these resources, for a single row's Protect.
	Only []protectPlanItem `json:"only,omitempty"`
	// Apply false returns the plan and changes nothing.
	Apply bool `json:"apply"`
}

type protectPlanItem struct {
	Kind    string `json:"kind"`
	AppName string `json:"app_name,omitempty"`
	Name    string `json:"name"`
	Scratch bool   `json:"scratch,omitempty"`
}

type protectFailure struct {
	protectPlanItem
	Error string `json:"error"`
}

type protectAllResponse struct {
	Applied    bool              `json:"applied"`
	TargetID   string            `json:"target_id"`
	Schedule   string            `json:"schedule"`
	Retain     int               `json:"retain"`
	RetainDays int               `json:"retain_days"`
	Plan       []protectPlanItem `json:"plan"`
	Done       []protectPlanItem `json:"done,omitempty"`
	Failed     []protectFailure  `json:"failed,omitempty"`
}

// handleProtectAll schedules backups for every unprotected database and volume.
// The plan is recomputed on apply, so a stale preview cannot widen the change.
func (rt *Router) handleProtectAll(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "backup coverage is not configured on this control plane")
		return
	}
	var req protectAllRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	def := currentBackupDefaults()
	if req.Schedule == "" {
		req.Schedule = def.Schedule
	}
	retain, retainDays := def.Retain, def.RetainDays
	if req.Retain != nil {
		retain = *req.Retain
	}
	if req.RetainDays != nil {
		retainDays = *req.RetainDays
	}
	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}
	if !validateBackupScheduleRequest(w, req.TargetID, req.Schedule, retain, retainDays) {
		return
	}
	if !rt.loadBackupTarget(w, r, req.TargetID, "api: protect all: load backup target failed") {
		return
	}

	rows, _, err := rt.buildCoverage(r.Context())
	if err != nil {
		rt.internalError(w, "api: protect all: coverage failed", err)
		return
	}
	resp := protectAllResponse{TargetID: req.TargetID, Schedule: req.Schedule, Retain: retain, RetainDays: retainDays, Plan: []protectPlanItem{}}
	only := make(map[protectPlanItem]bool, len(req.Only))
	for _, o := range req.Only {
		o.Scratch = false
		only[o] = true
	}
	allowed, err := rt.authorizeRefPredicate(r, AbilityWriteSensitive)
	if err != nil {
		rt.internalError(w, "api: protect all: authorize", err)
		return
	}
	for _, c := range rows {
		if c.State != backup.CoverageUnprotected {
			continue
		}
		item := protectPlanItem{Kind: c.Kind, AppName: c.AppName, Name: c.Name}
		ref := appRef(c.AppName)
		if c.Kind == store.BackupResourceKindDatabase {
			ref = databaseRef(c.Name)
		}
		if ok, err := allowed(ref); err != nil {
			rt.internalError(w, "api: protect all: authorize", err)
			return
		} else if !ok {
			continue
		}
		if len(only) > 0 {
			if !only[item] {
				continue
			}
		} else if c.Scratch && !req.IncludeScratch {
			continue
		}
		item.Scratch = c.Scratch
		resp.Plan = append(resp.Plan, item)
	}
	if !req.Apply {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Applied = true
	for _, item := range resp.Plan {
		if err := rt.protectOne(r.Context(), item, req.TargetID, req.Schedule, retain, retainDays); err != nil {
			rt.logger.Error("api: protect all: schedule failed", slog.String("kind", item.Kind), slog.String("app", item.AppName), slog.String("name", item.Name), slog.String("error", err.Error()))
			resp.Failed = append(resp.Failed, protectFailure{protectPlanItem: item, Error: "could not schedule this backup"})
			continue
		}
		resp.Done = append(resp.Done, item)
	}
	rt.logger.Info("api: protect all applied", slog.Int("scheduled", len(resp.Done)), slog.Int("failed", len(resp.Failed)), slog.String("target_id", req.TargetID))
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) protectOne(ctx context.Context, item protectPlanItem, targetID, schedule string, retain, retainDays int) error {
	if item.Kind == store.BackupResourceKindDatabase {
		return rt.databases.SetDatabaseBackupSchedule(ctx, item.Name, targetID, schedule, retain, retainDays)
	}
	sqlitePath := ""
	cfg, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(ctx, item.AppName, item.Name)
	if err == nil {
		sqlitePath = cfg.SqlitePath
	} else if !errors.Is(err, store.ErrServiceVolumeBackupNotFound) {
		return fmt.Errorf("read existing volume backup config: %w", err)
	}
	return rt.serviceVolumeBackupSchedule.SetServiceVolumeBackupSchedule(ctx, item.AppName, item.Name, targetID, schedule, retain, retainDays, sqlitePath)
}
