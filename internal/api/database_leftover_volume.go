package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	existingVolumeReuse   = "reuse"
	existingVolumeDiscard = "discard"

	errCodeExistingVolume = "existing_volume"
)

// existingVolumeConflict is the 409 body for creating a database whose name
// still has a data volume from a deleted one: the client must choose.
type existingVolumeConflict struct {
	Error   string               `json:"error"`
	Code    string               `json:"code"`
	Volumes []existingVolumeInfo `json:"volumes"`
}

type existingVolumeInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt string `json:"created_at,omitempty"`
	Mounted   bool   `json:"mounted"`
}

// databaseVolumeNames mirrors internal/reconcile/database's volume naming,
// the same duplication desiredVolumeNames documents.
func databaseVolumeNames(dbName string) (data string, all []string) {
	data = "db-" + dbName + "-data"
	return data, []string{data, "db-" + dbName + "-certs", "db-" + dbName + "-wal-archive"}
}

// leftoverDatabaseVolumes returns the volumes a deleted database of this name
// left on the control plane's own node. Remote nodes are not inspected.
func (rt *Router) leftoverDatabaseVolumes(ctx context.Context, dbName, nodeID string) ([]docker.NamedVolume, error) {
	if rt.orphanedVolumes == nil || (nodeID != "" && nodeID != rt.localNodeID) {
		return nil, nil
	}
	all, err := rt.orphanedVolumes.ListNamedVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	_, names := databaseVolumeNames(dbName)
	var found []docker.NamedVolume
	for _, v := range all {
		for _, n := range names {
			if v.Name == n {
				found = append(found, v)
			}
		}
	}
	return found, nil
}

// resolveExistingVolume applies the caller's explicit choice about leftover
// volumes, writing the response and returning false when creation must stop.
func (rt *Router) resolveExistingVolume(w http.ResponseWriter, r *http.Request, req databaseResource) bool {
	found, err := rt.leftoverDatabaseVolumes(r.Context(), req.Name, req.NodeID)
	if err != nil {
		rt.internalError(w, "api: create database: check leftover volumes failed", err)
		return false
	}
	dataName, _ := databaseVolumeNames(req.Name)
	hasData := false
	for _, v := range found {
		if v.Name == dataName {
			hasData = true
		}
	}
	if !hasData {
		return true
	}

	switch req.ExistingVolume {
	case existingVolumeReuse:
		return true
	case existingVolumeDiscard:
		for _, v := range found {
			if v.Mounted {
				writeError(w, http.StatusConflict, "volume "+v.Name+" is still attached to a container; wait for the old database to stop and retry")
				return false
			}
		}
		for _, v := range found {
			if err := rt.orphanedVolumes.RemoveVolume(r.Context(), v.Name); err != nil {
				rt.internalError(w, "api: create database: discard leftover volume failed", err)
				return false
			}
		}
		return true
	case "":
		infos := make([]existingVolumeInfo, 0, len(found))
		for _, v := range found {
			infos = append(infos, existingVolumeInfo{Name: v.Name, SizeBytes: v.SizeBytes, CreatedAt: v.CreatedAt, Mounted: v.Mounted})
		}
		writeJSON(w, http.StatusConflict, existingVolumeConflict{
			Error: "a deleted database named " + req.Name + " left its data volume behind: choose existing_volume \"reuse\" to attach the old data or \"discard\" to delete it and start empty (CLI: --existing-volume)",
			Code:  errCodeExistingVolume, Volumes: infos,
		})
		return false
	default:
		writeError(w, http.StatusBadRequest, "existing_volume must be \"reuse\" or \"discard\", got \""+strings.TrimSpace(req.ExistingVolume)+"\"")
		return false
	}
}
