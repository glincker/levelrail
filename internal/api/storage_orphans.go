package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envOrphanAttentionBytes     = "APP_ATTENTION_ORPHAN_BYTES"
	defaultOrphanAttentionBytes = int64(1) << 30

	orphanHistoryWindow = 5000
)

// orphanOwner is the resource an orphaned volume belonged to, as far as
// the volume name and backup history can tell.
type orphanOwner struct {
	Kind    string `json:"kind"`
	AppName string `json:"app_name,omitempty"`
	Name    string `json:"name,omitempty"`
	Role    string `json:"role"`
}

type storageOrphanResource struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	SizeBytes   *int64                 `json:"size_bytes,omitempty"`
	CreatedAt   string                 `json:"created_at,omitempty"`
	AgeDays     *int                   `json:"age_days,omitempty"`
	BelongedTo  orphanOwner            `json:"belonged_to"`
	BackupCount int                    `json:"backup_count"`
	LastBackup  *backupSummaryResource `json:"last_backup,omitempty"`
}

type storageOrphansResponse struct {
	Orphans    []storageOrphanResource `json:"orphans"`
	TotalBytes int64                   `json:"total_bytes"`
	// ThresholdBytes is the size above which the dashboard recommends a
	// clean-up.
	ThresholdBytes int64     `json:"threshold_bytes"`
	Page           *pageInfo `json:"page,omitempty"`
}

func orphanAttentionThreshold() int64 {
	if n, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(envOrphanAttentionBytes)), 10, 64); err == nil && n > 0 {
		return n
	}
	return defaultOrphanAttentionBytes
}

// dbVolumeSuffixes are the volume roles a managed database creates, longest
// first so "-wal-archive" is not read as part of a database name.
var dbVolumeSuffixes = []struct{ suffix, role string }{
	{"-wal-archive", "wal archive"},
	{"-certs", "certificates"},
	{"-data", "data"},
}

// classifyOrphanVolume names the owner of a leftover volume. A database
// volume is read from its name; an app volume is matched against backup
// history because "app-<service>-<volume>" is ambiguous on its own.
func classifyOrphanVolume(name string, appVolumes map[string]orphanOwner) orphanOwner {
	if rest, ok := strings.CutPrefix(name, "db-"); ok {
		for _, s := range dbVolumeSuffixes {
			if db, ok := strings.CutSuffix(rest, s.suffix); ok && db != "" {
				return orphanOwner{Kind: store.BackupResourceKindDatabase, Name: db, Role: s.role}
			}
		}
		return orphanOwner{Kind: store.BackupResourceKindDatabase, Name: rest, Role: "volume"}
	}
	if o, ok := appVolumes[name]; ok {
		return o
	}
	return orphanOwner{Kind: "app", Name: strings.TrimPrefix(name, "app-"), Role: "volume"}
}

type orphanBackupInfo struct {
	count int
	last  *backupSummaryResource
}

func (rt *Router) orphanBackupIndex(ctx context.Context) (volumes map[string]orphanOwner, dbs map[string]orphanBackupInfo, vols map[string]orphanBackupInfo, err error) {
	history, err := rt.backupHistory.ListAllBackupHistory(ctx, orphanHistoryWindow, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list backup history: %w", err)
	}
	volumes, dbs, vols = map[string]orphanOwner{}, map[string]orphanBackupInfo{}, map[string]orphanBackupInfo{}
	for _, h := range history {
		var info orphanBackupInfo
		var key string
		isVolume := h.ResourceKind == store.BackupResourceKindVolume
		if isVolume {
			key = "app-" + h.ServiceName + "-" + h.VolumeName
			volumes[key] = orphanOwner{Kind: store.BackupResourceKindVolume, AppName: h.ServiceName, Name: h.VolumeName, Role: "volume"}
			info = vols[key]
		} else {
			key = h.DatabaseName
			info = dbs[key]
		}
		if h.Status == store.BackupStatusSucceeded {
			info.count++
			if info.last == nil {
				s := &backupSummaryResource{ID: h.ID, At: h.StartedAt, Status: h.Status, SizeBytes: h.SizeBytes, Codec: h.Codec}
				info.last = s
			}
		}
		if isVolume {
			vols[key] = info
		} else {
			dbs[key] = info
		}
	}
	return volumes, dbs, vols, nil
}

func (rt *Router) describeOrphans(ctx context.Context, now time.Time) ([]storageOrphanResource, int64, error) {
	orphans, err := rt.computeOrphanedVolumes(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]storageOrphanResource, 0, len(orphans))
	if len(orphans) == 0 {
		return out, 0, nil
	}
	appVols, dbBackups, volBackups, err := rt.orphanBackupIndex(ctx)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	for _, v := range orphans {
		res := storageOrphanResource{ID: v.Name, Name: v.Name, CreatedAt: v.CreatedAt, BelongedTo: classifyOrphanVolume(v.Name, appVols)}
		if v.SizeBytes >= 0 {
			size := v.SizeBytes
			res.SizeBytes = &size
			total += size
		}
		if t, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
			days := int(now.Sub(t).Hours() / 24)
			res.AgeDays = &days
		}
		info := volBackups["app-"+res.BelongedTo.AppName+"-"+res.BelongedTo.Name]
		if res.BelongedTo.Kind == store.BackupResourceKindDatabase {
			info = dbBackups[res.BelongedTo.Name]
		}
		res.BackupCount, res.LastBackup = info.count, info.last
		out = append(out, res)
	}
	return out, total, nil
}

// handleListStorageOrphans handles GET /api/v1/storage/orphans: Docker
// volumes this control plane created whose database or app no longer
// exists, with size, age and the last backup of what they held.
func (rt *Router) handleListStorageOrphans(w http.ResponseWriter, r *http.Request) {
	if rt.orphanedVolumes == nil {
		writeError(w, http.StatusNotImplemented, "docker volume management is not configured on this control plane")
		return
	}
	list, total, err := rt.describeOrphans(r.Context(), time.Now())
	if err != nil {
		rt.internalError(w, "api: list storage orphans failed", err)
		return
	}
	resp := storageOrphansResponse{TotalBytes: total, ThresholdBytes: orphanAttentionThreshold()}
	resp.Orphans, resp.Page = pageWithInfo(w, r, list)
	writeJSON(w, http.StatusOK, resp)
}

type reclaimOrphanRequest struct {
	Confirm string `json:"confirm"`
}

type reclaimOrphanResponse struct {
	Removed        string `json:"removed"`
	ReclaimedBytes int64  `json:"reclaimed_bytes"`
}

// handleReclaimStorageOrphan handles DELETE /api/v1/storage/orphans/{id}.
// The caller must type the volume name back. Orphan status is recomputed
// here, so a volume an app started using again meanwhile is refused.
func (rt *Router) handleReclaimStorageOrphan(w http.ResponseWriter, r *http.Request) {
	if rt.orphanedVolumes == nil {
		writeError(w, http.StatusNotImplemented, "docker volume management is not configured on this control plane")
		return
	}
	id := r.PathValue("id")
	var req reclaimOrphanRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if req.Confirm == "" {
		req.Confirm = r.URL.Query().Get("confirm")
	}
	if req.Confirm != id {
		writeError(w, http.StatusBadRequest, "confirm must be the exact volume name: this deletes its data and cannot be undone")
		return
	}
	orphans, err := rt.computeOrphanedVolumes(r.Context())
	if err != nil {
		rt.internalError(w, "api: reclaim storage orphan: compute failed", err, slog.String("volume", id))
		return
	}
	var target *docker.NamedVolume
	for i := range orphans {
		if orphans[i].Name == id {
			target = &orphans[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "volume is not orphaned data (it may be in use again, or already removed)")
		return
	}
	if err := rt.orphanedVolumes.RemoveVolume(r.Context(), id); err != nil {
		rt.internalError(w, "api: reclaim storage orphan failed", err, slog.String("volume", id))
		return
	}
	var reclaimed int64
	if target.SizeBytes > 0 {
		reclaimed = target.SizeBytes
	}
	rt.logger.Info("api: orphaned volume reclaimed", slog.String("volume", id), slog.Int64("bytes", reclaimed))
	writeJSON(w, http.StatusOK, reclaimOrphanResponse{Removed: id, ReclaimedBytes: reclaimed})
}

// purgeVolumes removes the named local volumes after their owner is gone.
// A volume still in use is skipped with a warning and stays visible as
// orphaned data.
func (rt *Router) purgeVolumes(ctx context.Context, owner string, names []string) {
	if rt.orphanedVolumes == nil {
		return
	}
	for _, n := range names {
		if err := rt.orphanedVolumes.RemoveVolume(ctx, n); err != nil && !errors.Is(err, context.Canceled) {
			rt.logger.Warn("api: delete data: volume not removed, it stays listed as orphaned data", slog.String("owner", owner), slog.String("volume", n), slog.String("error", err.Error()))
			continue
		}
		rt.logger.Info("api: delete data: volume removed", slog.String("owner", owner), slog.String("volume", n))
	}
}

// appDeleteDataPlan resolves ?delete_data for an app delete: whether to purge
// and which Docker volumes belong to it. A missing app is left for deleteApp
// to answer 404.
func (rt *Router) appDeleteDataPlan(w http.ResponseWriter, r *http.Request, name string) (purge bool, volumes []string, ok bool) {
	if r.URL.Query().Get("delete_data") != "true" {
		return false, nil, true
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		return false, nil, true
	}
	if err != nil {
		rt.internalError(w, "api: delete app: load failed", err, slog.String("name", name))
		return false, nil, false
	}
	purge, ok = rt.requireDeleteDataConfirm(w, r, name, svc.NodeID, false)
	if !ok {
		return false, nil, false
	}
	for _, v := range svc.Volumes {
		volumes = append(volumes, v.Name)
	}
	return purge, volumes, true
}

// requireDeleteDataConfirm validates ?delete_data=true on a database or app
// delete: the typed confirmation must equal the resource name, the caller
// must hold write:sensitive, and the resource must live on this node because
// only the local Docker daemon is managed here. It writes the error itself.
func (rt *Router) requireDeleteDataConfirm(w http.ResponseWriter, r *http.Request, name, nodeID string, remoteOK bool) (purge, ok bool) {
	if r.URL.Query().Get("delete_data") != "true" {
		return false, true
	}
	if !remoteOK && !rt.isLocalNode(nodeID) {
		writeError(w, http.StatusBadRequest, "delete_data only works for resources on the control plane's own node; delete without it and remove the volume on that node")
		return false, false
	}
	if r.URL.Query().Get("confirm") != name {
		writeError(w, http.StatusBadRequest, "delete_data needs confirm set to the exact resource name: the data is destroyed and cannot be recovered")
		return false, false
	}
	if !rt.callerHasAbility(r, AbilityWriteSensitive) {
		writeError(w, http.StatusForbidden, "deleting data needs the write:sensitive ability")
		return false, false
	}
	return true, true
}
