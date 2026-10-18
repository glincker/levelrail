package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	storageKindS3    = "s3"
	storageKindAgent = "agent"

	defaultAgentDirectory = "backups"
)

// Data classes a destination can be the default for. Node directories only serve backups and dumps because the rest needs an S3 API.
const (
	StorageClassBackups   = "backups"
	StorageClassArchives  = "archives"
	StorageClassArtifacts = "artifacts"
	StorageClassCache     = "cache"
	StorageClassDumps     = "dumps"
)

var storageDataClasses = []string{StorageClassBackups, StorageClassArchives, StorageClassArtifacts, StorageClassCache, StorageClassDumps}

var agentDirPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)

// StoragePolicyStore persists per-class default destinations and reports how much backup data each destination holds.
type StoragePolicyStore interface {
	SetStorageDefault(ctx context.Context, dataClass, targetID, updatedAt string) error
	ClearStorageDefault(ctx context.Context, dataClass string) error
	ListStorageDefaults(ctx context.Context) ([]store.StorageDefault, error)
	StorageUsageByTarget(ctx context.Context) (map[string]store.StorageUsage, error)
}

func quotaOrZero(q *int64) int64 {
	if q == nil {
		return 0
	}
	return *q
}

func storageClassAllowedOnAgent(class string) bool {
	return class == StorageClassBackups || class == StorageClassDumps
}

func validStorageClass(class string) bool {
	for _, c := range storageDataClasses {
		if c == class {
			return true
		}
	}
	return false
}

type storageDecorations struct {
	usage    map[string]store.StorageUsage
	defaults map[string][]string
}

func (rt *Router) storageDecorations(ctx context.Context) storageDecorations {
	dec := storageDecorations{usage: map[string]store.StorageUsage{}, defaults: map[string][]string{}}
	if rt.storage == nil || rt.storage.Policy == nil {
		return dec
	}
	usage, err := rt.storage.Policy.StorageUsageByTarget(ctx)
	if err != nil {
		rt.logger.Warn("api: storage usage failed", slog.String("error", err.Error()))
	} else {
		dec.usage = usage
	}
	defaults, err := rt.storage.Policy.ListStorageDefaults(ctx)
	if err != nil {
		rt.logger.Warn("api: storage defaults failed", slog.String("error", err.Error()))
	}
	for _, d := range defaults {
		dec.defaults[d.TargetID] = append(dec.defaults[d.TargetID], d.DataClass)
	}
	return dec
}

func (dec storageDecorations) apply(res *storageDestinationResource) {
	u := dec.usage[res.ID]
	res.UsedBytes, res.Objects = u.Bytes, u.Objects
	res.OverQuota = res.QuotaBytes > 0 && u.Bytes > res.QuotaBytes
	res.MonthlyCostUSD = float64(u.Bytes) / 1e9 * objectstore.PricePerGBMonth(res.Preset, os.LookupEnv)
	if classes := dec.defaults[res.ID]; len(classes) > 0 {
		res.DefaultFor = classes
	}
}

// cleanAgentDirectory validates the directory a node destination writes into, relative to the node's storage root.
func cleanAgentDirectory(dir string) (string, error) {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	if dir == "" {
		return defaultAgentDirectory, nil
	}
	if path.Clean(dir) != dir || !agentDirPattern.MatchString(dir) || strings.Contains(dir, "..") {
		return "", errors.New("directory must be a plain relative path of letters, digits, dot, dash and underscore")
	}
	return dir, nil
}

func (rt *Router) agentNodeOrError(ctx context.Context, nodeID string) (int, string) {
	if rt.nodes == nil {
		return http.StatusNotImplemented, "nodes are not configured on this control plane"
	}
	n, err := rt.nodes.GetNode(ctx, nodeID)
	if err != nil || n == nil {
		return http.StatusBadRequest, "node not found: pick an enrolled node"
	}
	return 0, ""
}

func (rt *Router) createAgentStorageDestination(w http.ResponseWriter, r *http.Request, d *StorageDeps, req storageDestinationRequest) {
	req.Name = strings.TrimSpace(req.Name)
	req.NodeID = strings.TrimSpace(req.NodeID)
	dir, err := cleanAgentDirectory(req.Directory)
	if err == nil && req.Name == "" {
		err = errors.New("name is required")
	}
	if err == nil && req.NodeID == "" {
		err = errors.New("node_id is required")
	}
	if err == nil && req.QuotaBytes != nil && *req.QuotaBytes < 0 {
		err = errors.New("quota_bytes must not be negative")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if status, msg := rt.agentNodeOrError(r.Context(), req.NodeID); status != 0 {
		writeError(w, status, msg)
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusBadRequest, "unknown node_id"), nodeRef(req.NodeID)) {
		return
	}
	id, err := randomBackupTargetID()
	if err != nil {
		rt.internalError(w, "create storage destination: generate id", err)
		return
	}
	target := store.BackupTarget{ID: id, Name: req.Name, Provider: store.BackupProviderCustom, Endpoint: store.AgentEndpointScheme + req.NodeID, Bucket: dir, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.backupTargets.SaveBackupTarget(r.Context(), target); err != nil {
		rt.internalError(w, "create storage destination: save", err)
		return
	}
	opts := store.StorageOptions{TargetID: id, Preset: storageKindAgent, PathStyle: false, QuotaBytes: quotaOrZero(req.QuotaBytes)}
	if err := d.Options.SaveStorageOptions(r.Context(), opts); err != nil {
		rt.internalError(w, "create storage destination: save options", err)
		return
	}
	if req.Verify {
		if probe := rt.probeAgentDestination(r.Context(), id); !probe.OK {
			if derr := rt.backupTargets.DeleteBackupTarget(r.Context(), id); derr != nil {
				rt.logger.Warn("api: remove unverified storage destination failed", slog.String("error", derr.Error()), slog.String("id", id))
			}
			writeError(w, http.StatusBadRequest, "connection test failed ("+probe.Reason+"): "+probe.Message)
			return
		}
	}
	res := rt.toStorageResource(r.Context(), d, target, opts, true)
	rt.storageDecorations(r.Context()).apply(&res)
	writeJSON(w, http.StatusCreated, res)
}

func (rt *Router) updateAgentStorageDestination(w http.ResponseWriter, r *http.Request, d *StorageDeps, existing store.BackupTarget, prev store.StorageOptions, req storageDestinationRequest) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = existing.Name
	}
	node := strings.TrimPrefix(existing.Endpoint, store.AgentEndpointScheme)
	dir := existing.Bucket
	if strings.TrimSpace(req.Directory) != "" {
		var err error
		if dir, err = cleanAgentDirectory(req.Directory); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	quota := prev.QuotaBytes
	if req.QuotaBytes != nil {
		if *req.QuotaBytes < 0 {
			writeError(w, http.StatusBadRequest, "quota_bytes must not be negative")
			return
		}
		quota = *req.QuotaBytes
	}
	if err := rt.backupTargets.UpdateBackupTarget(r.Context(), existing.ID, name, existing.Provider, existing.Endpoint, "", dir); err != nil {
		rt.internalError(w, "update storage destination", err)
		return
	}
	opts := store.StorageOptions{TargetID: existing.ID, Preset: storageKindAgent, QuotaBytes: quota}
	if err := d.Options.SaveStorageOptions(r.Context(), opts); err != nil {
		rt.internalError(w, "update storage destination: save options", err)
		return
	}
	updated := existing
	updated.Name, updated.Bucket, updated.Endpoint = name, dir, store.AgentEndpointScheme+node
	res := rt.toStorageResource(r.Context(), d, updated, opts, true)
	rt.storageDecorations(r.Context()).apply(&res)
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) probeAgentDestination(ctx context.Context, id string) objectstore.ProbeResult {
	if rt.backupTargetTester == nil {
		return objectstore.ProbeResult{Reason: objectstore.ReasonUnknown, Message: "destination testing is not configured on this control plane", Steps: []objectstore.ProbeStep{}}
	}
	ctx, cancel := context.WithTimeout(ctx, storageProbeTimeout)
	defer cancel()
	err := rt.backupTargetTester.TestTarget(ctx, id)
	if err == nil {
		return objectstore.ProbeResult{OK: true, Steps: []objectstore.ProbeStep{{Name: "put", OK: true}, {Name: "get", OK: true}, {Name: "delete", OK: true}}}
	}
	reason, msg := classifyNodeError(err)
	return objectstore.ProbeResult{Reason: reason, Message: msg, Fix: objectstore.FixFor(reason), Steps: []objectstore.ProbeStep{{Name: "put", Error: msg}}}
}

func classifyNodeError(err error) (string, string) {
	text := err.Error()
	switch {
	case strings.Contains(text, "is not connected"):
		return objectstore.ReasonNodeOffline, "the node is not connected to the control plane right now: bring it online, then test again"
	case strings.Contains(text, "file storage is not enabled"):
		return objectstore.ReasonNodeStorageOff, "the node has no storage directory: set APP_AGENT_STORAGE_DIR on the agent and restart it"
	case strings.Contains(text, "escapes the storage directory"):
		return objectstore.ReasonUnknown, "the directory resolves outside the node's storage root"
	}
	return objectstore.ReasonUnknown, "node storage request failed: " + truncateStorageError(text)
}

func truncateStorageError(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

// suggestPathStyle turns a vague not-found or unreachable answer from a custom endpoint into a path-style hint when path-style addressing is off.
func suggestPathStyle(p objectstore.ProbeResult, endpoint string, pathStyle bool) objectstore.ProbeResult {
	if p.OK || pathStyle || endpoint == "" {
		return p
	}
	if p.Reason != objectstore.ReasonBucketNotFound && p.Reason != objectstore.ReasonUnreachable {
		return p
	}
	p.Reason = objectstore.ReasonPathStyle
	p.Message = "this endpoint probably needs path-style addressing (https://host/bucket/key): turn it on and test again"
	p.Fix = objectstore.FixFor(p.Reason)
	return p
}

type storageDefaultResource struct {
	DataClass string `json:"data_class"`
	TargetID  string `json:"target_id"`
	UpdatedAt string `json:"updated_at"`
}

type storageDefaultsResponse struct {
	Classes  []string                 `json:"classes"`
	Defaults []storageDefaultResource `json:"defaults"`
}

func (rt *Router) storagePolicyOrNotImplemented(w http.ResponseWriter) (*StorageDeps, bool) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return nil, false
	}
	if d.Policy == nil {
		writeError(w, http.StatusNotImplemented, "storage defaults are not configured on this control plane")
		return nil, false
	}
	return d, true
}

// handleListStorageDefaults handles GET /api/v1/storage/defaults.
func (rt *Router) handleListStorageDefaults(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storagePolicyOrNotImplemented(w)
	if !ok {
		return
	}
	rows, err := d.Policy.ListStorageDefaults(r.Context())
	if err != nil {
		rt.internalError(w, "list storage defaults", err)
		return
	}
	out := storageDefaultsResponse{Classes: storageDataClasses, Defaults: make([]storageDefaultResource, 0, len(rows))}
	for _, row := range rows {
		out.Defaults = append(out.Defaults, storageDefaultResource{DataClass: row.DataClass, TargetID: row.TargetID, UpdatedAt: row.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetStorageDefault handles PUT /api/v1/storage/defaults/{class}.
func (rt *Router) handleSetStorageDefault(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storagePolicyOrNotImplemented(w)
	if !ok {
		return
	}
	class := r.PathValue("class")
	if !validStorageClass(class) {
		writeError(w, http.StatusBadRequest, "unknown data class: use one of "+strings.Join(storageDataClasses, ", "))
		return
	}
	var req struct {
		TargetID string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.TargetID) == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}
	target, ok := rt.loadStorageTarget(w, r, req.TargetID)
	if !ok {
		return
	}
	if strings.HasPrefix(target.Endpoint, store.AgentEndpointScheme) && !storageClassAllowedOnAgent(class) {
		writeError(w, http.StatusBadRequest, "a node directory can only be the default for backups and dumps; "+class+" needs an S3 compatible destination")
		return
	}
	if err := d.Policy.SetStorageDefault(r.Context(), class, target.ID, time.Now().UTC().Format(time.RFC3339)); err != nil {
		rt.internalError(w, "set storage default", err)
		return
	}
	writeJSON(w, http.StatusOK, storageDefaultResource{DataClass: class, TargetID: target.ID, UpdatedAt: time.Now().UTC().Format(time.RFC3339)})
}

// handleClearStorageDefault handles DELETE /api/v1/storage/defaults/{class}.
func (rt *Router) handleClearStorageDefault(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storagePolicyOrNotImplemented(w)
	if !ok {
		return
	}
	class := r.PathValue("class")
	if !validStorageClass(class) {
		writeError(w, http.StatusBadRequest, "unknown data class: use one of "+strings.Join(storageDataClasses, ", "))
		return
	}
	if err := d.Policy.ClearStorageDefault(r.Context(), class); err != nil {
		rt.internalError(w, "clear storage default", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
