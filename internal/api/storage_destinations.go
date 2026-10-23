package api

import (
	"context"
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

	// Policy enables per-class defaults, usage and quota reporting; nil answers 501 for defaults and omits usage.
	Policy StoragePolicyStore

	// BuildCache and BuildCacheSettings enable /api/v1/build-cache; nil answers 501.
	BuildCache         BuildCacheService
	BuildCacheSettings BuildCacheSettingsStore

	// Buckets enables /api/v1/storage/buckets; nil answers 501. BucketSweeper runs lifecycle rules on demand.
	Buckets       BucketRowStore
	BucketSweeper BucketSweepService
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

	Kind           string   `json:"kind"`
	NodeID         string   `json:"node_id,omitempty"`
	Directory      string   `json:"directory,omitempty"`
	QuotaBytes     int64    `json:"quota_bytes"`
	UsedBytes      int64    `json:"used_bytes"`
	Objects        int64    `json:"objects"`
	OverQuota      bool     `json:"over_quota"`
	MonthlyCostUSD float64  `json:"monthly_cost_usd"`
	DefaultFor     []string `json:"default_for"`
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

	// Kind is "s3" (default) or "agent". An agent destination needs NodeID and Directory, no credentials.
	Kind       string `json:"kind,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
	Directory  string `json:"directory,omitempty"`
	QuotaBytes *int64 `json:"quota_bytes,omitempty"`
}

func (rt *Router) toStorageResource(ctx context.Context, d *StorageDeps, t store.BackupTarget, opts store.StorageOptions, hasOpts bool) storageDestinationResource {
	if !hasOpts {
		opts = store.StorageOptions{Preset: objectstore.PresetForProvider(t.Provider), PathStyle: t.Endpoint != ""}
	}
	n, err := d.Archive.CountLogArchivePoliciesForTarget(ctx, t.ID)
	if err != nil {
		rt.logger.Warn("api: count archive policies failed", slog.String("error", err.Error()), slog.String("id", t.ID))
	}
	res := storageDestinationResource{
		ID: t.ID, Name: t.Name, Preset: opts.Preset, Provider: t.Provider, Endpoint: t.Endpoint, Region: t.Region,
		Bucket: t.Bucket, PathStyle: opts.PathStyle, AccountID: opts.AccountID, CreatedAt: t.CreatedAt, ArchivePolicies: n,
		Kind: storageKindS3, QuotaBytes: opts.QuotaBytes, DefaultFor: []string{},
	}
	if node, ok := strings.CutPrefix(t.Endpoint, store.AgentEndpointScheme); ok {
		res.Kind, res.NodeID, res.Directory, res.Preset = storageKindAgent, node, t.Bucket, storageKindAgent
		res.Endpoint, res.Bucket = "", ""
	}
	return res
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
		rt.internalError(w, "api: list storage destinations failed", err)
		return
	}
	opts, err := d.Options.ListStorageOptions(r.Context())
	if err != nil {
		rt.internalError(w, "api: list storage options failed", err)
		return
	}
	dec := rt.storageDecorations(r.Context())
	out := make([]storageDestinationResource, 0, len(targets))
	for _, t := range targets {
		o, has := opts[t.ID]
		res := rt.toStorageResource(r.Context(), d, t, o, has)
		dec.apply(&res)
		out = append(out, res)
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
		rt.internalError(w, "api: get storage options failed", err, slog.String("id", t.ID))
		return
	}
	res := rt.toStorageResource(r.Context(), d, t, o, has)
	rt.storageDecorations(r.Context()).apply(&res)
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) loadStorageTarget(w http.ResponseWriter, r *http.Request, id string) (store.BackupTarget, bool) {
	t, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "storage destination not found")
		return store.BackupTarget{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load storage destination failed", err, slog.String("id", id))
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
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Kind == storageKindAgent {
		rt.createAgentStorageDestination(w, r, d, req)
		return
	}
	res, pathStyle, err := normalizeStorageRequest(&req)
	if err == nil && req.QuotaBytes != nil && *req.QuotaBytes < 0 {
		err = errors.New("quota_bytes must not be negative")
	}
	if err == nil && (req.AccessKeyID == "" || req.SecretAccessKey == "") {
		err = errors.New("access_key_id and secret_access_key are required")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Verify {
		if probe := suggestPathStyle(verifyStorageRequest(r.Context(), req, res, pathStyle), res.Endpoint, pathStyle); !probe.OK {
			writeError(w, http.StatusBadRequest, "connection test failed ("+probe.Reason+"): "+probe.Message)
			return
		}
	}

	preset, _ := objectstore.PresetByID(req.Preset)
	id, err := randomBackupTargetID()
	if err != nil {
		rt.internalError(w, "api: create storage destination: generate id failed", err)
		return
	}
	key := store.BackupTargetSecretsKey(id)
	for envKey, val := range map[string]string{"access_key_id": req.AccessKeyID, "secret_access_key": req.SecretAccessKey} {
		if err := rt.backupSecrets.SetValue(r.Context(), key, envKey, val); err != nil {
			rt.internalError(w, "api: create storage destination: store credential failed", err, slog.String("id", id), slog.String("key", envKey))
			return
		}
	}
	target := store.BackupTarget{ID: id, Name: req.Name, Provider: preset.Provider, Endpoint: res.Endpoint, Region: res.Region, Bucket: req.Bucket, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.backupTargets.SaveBackupTarget(r.Context(), target); err != nil {
		rt.internalError(w, "api: create storage destination: save failed", err, slog.String("id", id))
		return
	}
	opts := store.StorageOptions{TargetID: id, Preset: req.Preset, PathStyle: pathStyle, AccountID: strings.TrimSpace(req.AccountID), QuotaBytes: quotaOrZero(req.QuotaBytes)}
	if err := d.Options.SaveStorageOptions(r.Context(), opts); err != nil {
		rt.internalError(w, "api: create storage destination: save options failed", err, slog.String("id", id))
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
	if !decodeReq(w, r, &req) {
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusBadRequest, "unknown node_id"), nodeRef(strings.TrimSpace(req.NodeID))) {
		return
	}
	prev, _, err := d.Options.GetStorageOptions(r.Context(), existing.ID)
	if err != nil {
		rt.internalError(w, "api: update storage destination: load options failed", err, slog.String("id", existing.ID))
		return
	}
	if strings.HasPrefix(existing.Endpoint, store.AgentEndpointScheme) {
		rt.updateAgentStorageDestination(w, r, d, existing, prev, req)
		return
	}
	res, pathStyle, err := normalizeStorageRequest(&req)
	if err == nil && req.QuotaBytes != nil && *req.QuotaBytes < 0 {
		err = errors.New("quota_bytes must not be negative")
	}
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
				rt.internalError(w, "api: update storage destination: store credential failed", err, slog.String("id", existing.ID), slog.String("key", envKey))
				return
			}
		}
	}
	preset, _ := objectstore.PresetByID(req.Preset)
	if err := rt.backupTargets.UpdateBackupTarget(r.Context(), existing.ID, req.Name, preset.Provider, res.Endpoint, res.Region, req.Bucket); err != nil {
		rt.internalError(w, "api: update storage destination failed", err, slog.String("id", existing.ID))
		return
	}
	opts := store.StorageOptions{TargetID: existing.ID, Preset: req.Preset, PathStyle: pathStyle, AccountID: strings.TrimSpace(req.AccountID), QuotaBytes: prev.QuotaBytes}
	if req.QuotaBytes != nil {
		opts.QuotaBytes = *req.QuotaBytes
	}
	if err := d.Options.SaveStorageOptions(r.Context(), opts); err != nil {
		rt.internalError(w, "api: update storage destination: save options failed", err, slog.String("id", existing.ID))
		return
	}
	updated, ok := rt.loadStorageTarget(w, r, existing.ID)
	if !ok {
		return
	}
	out := rt.toStorageResource(r.Context(), d, updated, opts, true)
	rt.storageDecorations(r.Context()).apply(&out)
	writeJSON(w, http.StatusOK, out)
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
	target, ok := rt.loadStorageTarget(w, r, id)
	if !ok {
		return
	}
	if strings.HasPrefix(target.Endpoint, store.AgentEndpointScheme) {
		writeJSON(w, http.StatusOK, rt.probeAgentDestination(r.Context(), id))
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
	probe := objectstore.Probe(ctx, c)
	if opts, has, oerr := d.Options.GetStorageOptions(r.Context(), id); oerr == nil && has {
		probe = suggestPathStyle(probe, target.Endpoint, opts.PathStyle)
	}
	writeJSON(w, http.StatusOK, probe)
}
