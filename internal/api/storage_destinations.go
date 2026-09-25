package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

const storageProbeTimeout = 30 * time.Second

// StorageOptionsStore persists per-destination options.
type StorageOptionsStore interface {
	SaveStorageOptions(ctx context.Context, o store.StorageOptions) error
	GetStorageOptions(ctx context.Context, targetID string) (store.StorageOptions, bool, error)
	ListStorageOptions(ctx context.Context) (map[string]store.StorageOptions, error)
}

// LogArchiveStore persists log archive policies and run history.
type LogArchiveStore interface {
	UpsertLogArchivePolicy(ctx context.Context, p store.LogArchivePolicy) error
	GetLogArchivePolicy(ctx context.Context, appName string) (store.LogArchivePolicy, error)
	ListLogArchivePolicies(ctx context.Context) ([]store.LogArchivePolicy, error)
	DeleteLogArchivePolicy(ctx context.Context, appName string) error
	CountLogArchivePoliciesForTarget(ctx context.Context, targetID string) (int, error)
	ListLogArchiveRuns(ctx context.Context, appName string, all bool, limit int) ([]store.LogArchiveRun, error)
}

// ObjectClientResolver builds a bucket client for a stored destination.
type ObjectClientResolver interface {
	Client(ctx context.Context, targetID string) (*objectstore.Client, store.BackupTarget, error)
}

// LogArchiver starts manual log dumps.
type LogArchiver interface {
	StartDump(ctx context.Context, req objectstore.DumpRequest) (store.LogArchiveRun, error)
}

// StorageDeps groups what the storage destination and log archive routes need.
type StorageDeps struct {
	Options     StorageOptionsStore
	Archive     LogArchiveStore
	Clients     ObjectClientResolver
	Archiver    LogArchiver
	ArchiveRoot string
}

// WithStorage enables the /api/v1/storage and /api/v1/log-archive routes.
// Without it they return 501.
func WithStorage(d StorageDeps) Option {
	return func(rt *Router) { rt.storage = &d }
}

// SetStorage enables the storage routes after construction, for wiring that
// needs the built Router first. Call it before the server starts serving.
func (rt *Router) SetStorage(d StorageDeps) { rt.storage = &d }

func (rt *Router) storageOrNotImplemented(w http.ResponseWriter) (*StorageDeps, bool) {
	if rt.storage == nil {
		writeError(w, http.StatusNotImplemented, "storage destinations are not configured on this control plane")
		return nil, false
	}
	return rt.storage, true
}

type storageDestinationResource struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Preset          string `json:"preset"`
	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket"`
	PathStyle       bool   `json:"path_style"`
	AccountID       string `json:"account_id,omitempty"`
	CreatedAt       string `json:"created_at"`
	ArchivePolicies int    `json:"archive_policies"`
}

type storageDestinationRequest struct {
	Name            string `json:"name"`
	Preset          string `json:"preset"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket"`
	AccountID       string `json:"account_id,omitempty"`
	PathStyle       *bool  `json:"path_style,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	// Verify runs the put/get/delete probe before saving and rejects a failing destination.
	Verify bool `json:"verify,omitempty"`
}

func (rt *Router) toStorageResource(ctx context.Context, d *StorageDeps, t store.BackupTarget, opts store.StorageOptions, hasOpts bool) storageDestinationResource {
	if !hasOpts {
		opts = store.StorageOptions{Preset: objectstore.PresetForProvider(t.Provider), PathStyle: t.Endpoint != ""}
	}
	n, err := d.Archive.CountLogArchivePoliciesForTarget(ctx, t.ID)
	if err != nil {
		rt.logger.Warn("api: count archive policies failed", slog.String("error", err.Error()), slog.String("id", t.ID))
	}
	return storageDestinationResource{
		ID: t.ID, Name: t.Name, Preset: opts.Preset, Provider: t.Provider, Endpoint: t.Endpoint, Region: t.Region,
		Bucket: t.Bucket, PathStyle: opts.PathStyle, AccountID: opts.AccountID, CreatedAt: t.CreatedAt, ArchivePolicies: n,
	}
}

// handleListStorageProviders handles GET /api/v1/storage/providers.
func (rt *Router) handleListStorageProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, objectstore.Presets())
}

// handleListStorageDestinations handles GET /api/v1/storage/destinations.
func (rt *Router) handleListStorageDestinations(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	targets, err := rt.backupTargets.ListBackupTargets(r.Context())
	if err != nil {
		rt.logger.Error("api: list storage destinations failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	opts, err := d.Options.ListStorageOptions(r.Context())
	if err != nil {
		rt.logger.Error("api: list storage options failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]storageDestinationResource, 0, len(targets))
	for _, t := range targets {
		o, has := opts[t.ID]
		out = append(out, rt.toStorageResource(r.Context(), d, t, o, has))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetStorageDestination handles GET /api/v1/storage/destinations/{id}.
func (rt *Router) handleGetStorageDestination(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	t, ok := rt.loadStorageTarget(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	o, has, err := d.Options.GetStorageOptions(r.Context(), t.ID)
	if err != nil {
		rt.logger.Error("api: get storage options failed", slog.String("error", err.Error()), slog.String("id", t.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toStorageResource(r.Context(), d, t, o, has))
}

func (rt *Router) loadStorageTarget(w http.ResponseWriter, r *http.Request, id string) (store.BackupTarget, bool) {
	t, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "storage destination not found")
		return store.BackupTarget{}, false
	}
	if err != nil {
		rt.logger.Error("api: load storage destination failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return store.BackupTarget{}, false
	}
	return t, true
}

// verifyStorageRequest probes a destination that is not stored yet.
func verifyStorageRequest(ctx context.Context, req storageDestinationRequest, res objectstore.Resolved, pathStyle bool) objectstore.ProbeResult {
	ctx, cancel := context.WithTimeout(ctx, storageProbeTimeout)
	defer cancel()
	c, err := objectstore.New(objectstore.Config{
		Endpoint: res.Endpoint, Region: res.Region, Bucket: req.Bucket,
		AccessKeyID: req.AccessKeyID, SecretAccessKey: req.SecretAccessKey, PathStyle: pathStyle, MaxAttempts: 2,
	})
	if err != nil {
		return objectstore.ProbeResult{Reason: objectstore.ReasonUnknown, Message: err.Error(), Steps: []objectstore.ProbeStep{}}
	}
	return objectstore.Probe(ctx, c)
}

func normalizeStorageRequest(req *storageDestinationRequest) (objectstore.Resolved, bool, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Bucket = strings.TrimSpace(req.Bucket)
	if req.Name == "" {
		return objectstore.Resolved{}, false, errors.New("name is required")
	}
	if req.Bucket == "" {
		return objectstore.Resolved{}, false, errors.New("bucket is required")
	}
	if req.Preset == "" {
		req.Preset = objectstore.PresetCustom
	}
	res, err := objectstore.Resolve(req.Preset, req.Endpoint, req.Region, req.AccountID)
	if err != nil {
		return objectstore.Resolved{}, false, err
	}
	preset, _ := objectstore.PresetByID(req.Preset)
	pathStyle := preset.PathStyle
	if req.PathStyle != nil {
		pathStyle = *req.PathStyle
	}
	return res, pathStyle, nil
}

// handleCreateStorageDestination handles POST /api/v1/storage/destinations.
func (rt *Router) handleCreateStorageDestination(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	if rt.backupSecrets == nil {
		writeError(w, http.StatusNotImplemented, "storage destinations need a master key configured on this control plane")
		return
	}
	var req storageDestinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, pathStyle, err := normalizeStorageRequest(&req)
	if err == nil && (req.AccessKeyID == "" || req.SecretAccessKey == "") {
		err = errors.New("access_key_id and secret_access_key are required")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Verify {
		if probe := verifyStorageRequest(r.Context(), req, res, pathStyle); !probe.OK {
			writeError(w, http.StatusBadRequest, "connection test failed ("+probe.Reason+"): "+probe.Message)
			return
		}
	}

	preset, _ := objectstore.PresetByID(req.Preset)
	id, err := randomBackupTargetID()
	if err != nil {
		rt.logger.Error("api: create storage destination: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	key := store.BackupTargetSecretsKey(id)
	for envKey, val := range map[string]string{"access_key_id": req.AccessKeyID, "secret_access_key": req.SecretAccessKey} {
		if err := rt.backupSecrets.SetValue(r.Context(), key, envKey, val); err != nil {
			rt.logger.Error("api: create storage destination: store credential failed", slog.String("error", err.Error()), slog.String("id", id), slog.String("key", envKey))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	target := store.BackupTarget{ID: id, Name: req.Name, Provider: preset.Provider, Endpoint: res.Endpoint, Region: res.Region, Bucket: req.Bucket, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.backupTargets.SaveBackupTarget(r.Context(), target); err != nil {
		rt.logger.Error("api: create storage destination: save failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	opts := store.StorageOptions{TargetID: id, Preset: req.Preset, PathStyle: pathStyle, AccountID: strings.TrimSpace(req.AccountID)}
	if err := d.Options.SaveStorageOptions(r.Context(), opts); err != nil {
		rt.logger.Error("api: create storage destination: save options failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, rt.toStorageResource(r.Context(), d, target, opts, true))
}

// handleUpdateStorageDestination handles PUT /api/v1/storage/destinations/{id}.
func (rt *Router) handleUpdateStorageDestination(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	existing, ok := rt.loadStorageTarget(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req storageDestinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, pathStyle, err := normalizeStorageRequest(&req)
	if err == nil && (req.AccessKeyID == "") != (req.SecretAccessKey == "") {
		err = errors.New("access_key_id and secret_access_key must be set together to rotate credentials")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.AccessKeyID != "" {
		if rt.backupSecrets == nil {
			writeError(w, http.StatusNotImplemented, "storage destinations need a master key configured on this control plane")
			return
		}
		key := store.BackupTargetSecretsKey(existing.ID)
		for envKey, val := range map[string]string{"access_key_id": req.AccessKeyID, "secret_access_key": req.SecretAccessKey} {
			if err := rt.backupSecrets.SetValue(r.Context(), key, envKey, val); err != nil {
				rt.logger.Error("api: update storage destination: store credential failed", slog.String("error", err.Error()), slog.String("id", existing.ID), slog.String("key", envKey))
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
	}
	preset, _ := objectstore.PresetByID(req.Preset)
	if err := rt.backupTargets.UpdateBackupTarget(r.Context(), existing.ID, req.Name, preset.Provider, res.Endpoint, res.Region, req.Bucket); err != nil {
		rt.logger.Error("api: update storage destination failed", slog.String("error", err.Error()), slog.String("id", existing.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	opts := store.StorageOptions{TargetID: existing.ID, Preset: req.Preset, PathStyle: pathStyle, AccountID: strings.TrimSpace(req.AccountID)}
	if err := d.Options.SaveStorageOptions(r.Context(), opts); err != nil {
		rt.logger.Error("api: update storage destination: save options failed", slog.String("error", err.Error()), slog.String("id", existing.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	updated, ok := rt.loadStorageTarget(w, r, existing.ID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.toStorageResource(r.Context(), d, updated, opts, true))
}

// handleDeleteStorageDestination handles DELETE /api/v1/storage/destinations/{id}.
func (rt *Router) handleDeleteStorageDestination(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if n, err := d.Archive.CountLogArchivePoliciesForTarget(r.Context(), id); err == nil && n > 0 {
		writeError(w, http.StatusConflict, "storage destination is used by a log archive policy; remove the policy first")
		return
	}
	err := rt.backupTargets.DeleteBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "storage destination not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "storage destination still has backup history; delete is blocked until that is addressed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestStorageDestination handles POST /api/v1/storage/destinations/{id}/test.
// It answers 200 with the probe outcome even when the probe fails, so
// callers can show the reason and per-step results.
func (rt *Router) handleTestStorageDestination(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, ok := rt.loadStorageTarget(w, r, id); !ok {
		return
	}
	c, _, err := d.Clients.Client(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: test storage destination: resolve failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusBadGateway, "could not load the destination credentials")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), storageProbeTimeout)
	defer cancel()
	writeJSON(w, http.StatusOK, objectstore.Probe(ctx, c))
}
