package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/kit/ids"
)

// restoreToNodeHistoryScan bounds how many recent attempts are read to find the newest succeeded one.
const restoreToNodeHistoryScan = 25

type appRestoreToNodeRequest struct {
	NodeID string `json:"node_id"`
	// DryRun returns the plan without creating anything.
	DryRun bool `json:"dry_run,omitempty"`
}

type appRestoreToNodeItem struct {
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	NewName  string `json:"new_name,omitempty"`
	BackupID string `json:"backup_id,omitempty"`
	ID       string `json:"id,omitempty"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

type appRestoreToNodeResponse struct {
	App    string                 `json:"app"`
	NodeID string                 `json:"node_id"`
	DryRun bool                   `json:"dry_run"`
	Items  []appRestoreToNodeItem `json:"items"`
}

const (
	restoreItemPlanned = "planned"
	restoreItemStarted = "started"
	restoreItemSkipped = "skipped"
	restoreItemFailed  = "failed"
)

// handleAppRestoreToNode handles POST /api/v1/apps/{name}/restore-to-node: restores the newest
// succeeded backup of every volume and linked database of the app as brand-new resources on the chosen node.
// It never overwrites anything, so it needs the same ability as the single-resource restore-to routes.
func (rt *Router) handleAppRestoreToNode(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil || rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "restore to another node is not configured on this control plane")
		return
	}
	appName := r.PathValue("name")
	var req appRestoreToNodeRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.NodeID) == "" {
		writeError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), appName)
	if err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.internalError(w, "api: app restore-to-node: load app failed", err, slog.String("name", appName))
		return
	}
	dbNames := linkedDatabaseNames(svc)
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadGateway, "the chosen node is not currently reachable"), nodeRef(req.NodeID)) {
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusNotFound, "database not found"), databaseRefs(dbNames)...) {
		return
	}
	nodeRuntime, err := rt.execRuntime(req.NodeID)
	if err != nil {
		rt.logger.Error("api: app restore-to-node: resolve node runtime failed", slog.String("error", err.Error()), slog.String("node_id", req.NodeID))
		writeError(w, http.StatusBadGateway, "the chosen node is not currently reachable")
		return
	}

	resp := appRestoreToNodeResponse{App: appName, NodeID: req.NodeID, DryRun: req.DryRun, Items: []appRestoreToNodeItem{}}
	for _, v := range svc.Volumes {
		// Backup history is keyed by the logical name, not the prefixed Docker name.
		logical := strings.TrimPrefix(v.Name, store.ServiceVolumeName(appName, ""))
		item := appRestoreToNodeItem{Kind: "volume", Source: logical, Status: restoreItemPlanned}
		hist, err := rt.serviceVolumeBackupHistory.ListServiceVolumeBackupHistory(r.Context(), appName, logical, restoreToNodeHistoryScan, nil)
		if err != nil {
			rt.internalError(w, "api: app restore-to-node: list volume backups failed", err, slog.String("name", appName), slog.String("volume", logical))
			return
		}
		b, ok := newestSucceeded(hist)
		if !ok {
			item.Status, item.Reason = restoreItemSkipped, "no succeeded backup"
			resp.Items = append(resp.Items, item)
			continue
		}
		item.BackupID = b.ID
		if item.NewName, err = defaultVolumeCloneName(appName, logical); err != nil {
			rt.internalError(w, "api: app restore-to-node: generate volume name failed", err)
			return
		}
		if !req.DryRun {
			if item.ID, err = randomVolumeCloneRestoreID(); err != nil {
				rt.internalError(w, "api: app restore-to-node: generate id failed", err)
				return
			}
			item.Status = restoreItemStarted
			go func(it appRestoreToNodeItem, volName string) { //nolint:gosec // deliberately not r.Context(): cancelled when this handler returns
				if err := rt.backupProtection.RestoreVolumeTo(context.Background(), nodeRuntime, it.ID, appName, volName, it.NewName, it.BackupID, appName); err != nil {
					rt.logger.Error("api: app restore-to-node: volume restore failed", slog.String("error", err.Error()), slog.String("id", it.ID), slog.String("volume", volName))
				}
			}(item, logical)
		}
		resp.Items = append(resp.Items, item)
	}
	for _, dbName := range dbNames {
		resp.Items = append(resp.Items, rt.restoreDatabaseToNode(r, dbName, req))
	}

	status := http.StatusAccepted
	if req.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, resp)
}

// restoreDatabaseToNode plans or starts one database restore. Failures become item state so the volumes already started stay reported.
func (rt *Router) restoreDatabaseToNode(r *http.Request, dbName string, req appRestoreToNodeRequest) appRestoreToNodeItem {
	item := appRestoreToNodeItem{Kind: "database", Source: dbName, Status: restoreItemPlanned}
	skip := func(reason string) appRestoreToNodeItem {
		item.Status, item.Reason = restoreItemSkipped, reason
		return item
	}
	if rt.cloneRestoreRunner == nil || rt.backupHistory == nil {
		return skip("database restores are not configured")
	}
	src, err := rt.databases.GetDesiredDatabase(r.Context(), dbName)
	if err != nil {
		return skip("database not found")
	}
	if !store.EngineSupportsBackups(src.Engine) {
		return skip("engine keeps no backups")
	}
	hist, err := rt.backupHistory.ListBackupHistory(r.Context(), dbName, restoreToNodeHistoryScan, nil)
	if err != nil {
		return skip("backup history unavailable")
	}
	var chosen *store.BackupHistory
	for i := range hist {
		if hist[i].Status == store.BackupStatusSucceeded && src.OwnsBackup(hist[i].InstanceID, hist[i].StartedAt) {
			chosen = &hist[i]
			break
		}
	}
	if chosen == nil {
		return skip("no succeeded backup")
	}
	suffix, err := ids.Hex(6)
	if err != nil {
		return skip("could not generate a name")
	}
	item.BackupID, item.NewName = chosen.ID, dbName+"-dr-"+suffix
	if req.DryRun {
		return item
	}
	cw := &captureWriter{header: http.Header{}, status: http.StatusOK}
	newDB := databaseResource{Name: item.NewName, Engine: src.Engine, Version: src.Version, NodeID: req.NodeID}
	if !rt.createDesiredDatabase(cw, r, newDB) {
		item.Status, item.Reason = restoreItemFailed, fmt.Sprintf("create failed (%d): %s", cw.status, strings.TrimSpace(cw.body.String()))
		return item
	}
	rt.copyDatabaseTLSMode(r.Context(), dbName, item.NewName)
	if item.ID, err = randomCloneRestoreID(); err != nil {
		item.Status, item.Reason = restoreItemFailed, "could not generate an id"
		return item
	}
	item.Status = restoreItemStarted
	go func(it appRestoreToNodeItem, engine string) { //nolint:gosec // deliberately not r.Context(): cancelled when the handler returns
		if err := rt.cloneRestoreRunner.RunCloneRestore(context.Background(), it.ID, dbName, it.NewName, it.BackupID, engine, databaseContainerName(it.NewName), databaseControllerName(it.NewName)); err != nil {
			rt.logger.Error("api: app restore-to-node: database restore failed", slog.String("error", err.Error()), slog.String("id", it.ID), slog.String("database", dbName))
		}
	}(item, src.Engine)
	return item
}

// newestSucceeded expects history newest first, the order every history list returns.
func newestSucceeded(hist []store.BackupHistory) (store.BackupHistory, bool) {
	for _, h := range hist {
		if h.Status == store.BackupStatusSucceeded {
			return h, true
		}
	}
	return store.BackupHistory{}, false
}

// linkedDatabaseNames lists the managed databases the app's env resolves from, sorted and unique.
func linkedDatabaseNames(svc *store.DesiredService) []string {
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
