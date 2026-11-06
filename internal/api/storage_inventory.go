package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/storageinv"
)

// localNodeID is the node query value for the control plane's own daemon.
const localNodeID = "local"

type storageInventoryResponse struct {
	Node        string                 `json:"node"`
	Volumes     []storageinv.Volume    `json:"volumes"`
	Containers  []storageinv.Container `json:"containers"`
	Unsupported bool                   `json:"unsupported"`
	Reason      string                 `json:"reason,omitempty"`
}

// storageOwners resolves volume and database ownership from desired state.
func (rt *Router) storageOwners(ctx context.Context) (storageinv.Owners, error) {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return storageinv.Owners{}, fmt.Errorf("list desired services: %w", err)
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return storageinv.Owners{}, fmt.Errorf("list desired databases: %w", err)
	}
	referenced, err := rt.desiredVolumeNames(ctx)
	if err != nil {
		return storageinv.Owners{}, err
	}
	o := storageinv.Owners{
		VolumeApp:  map[string]string{},
		Databases:  make(map[string]bool, len(databases)),
		Referenced: referenced,
		ContainerApp: func(name string) string {
			if svc, ok := application.ParseContainerName(name); ok {
				return svc
			}
			return ""
		},
	}
	for _, svc := range services {
		for _, v := range svc.Volumes {
			o.VolumeApp[v.Name] = svc.Name
		}
	}
	for _, db := range databases {
		o.Databases[db.Name] = true
	}
	return o, nil
}

// handleStorageInventory handles GET /api/v1/system/storage/inventory: every
// volume and stopped container on a node with owner app and database flags.
// Read only, same AbilityRead tier as the orphan listings.
func (rt *Router) handleStorageInventory(w http.ResponseWriter, r *http.Request) {
	node := r.URL.Query().Get("node")
	if node != "" && node != localNodeID {
		writeJSON(w, http.StatusOK, storageInventoryResponse{
			Node: node, Volumes: []storageinv.Volume{}, Containers: []storageinv.Container{},
			Unsupported: true, Reason: "volume and container inventory is only available for the control plane's own node",
		})
		return
	}
	src, ok := rt.orphanedVolumes.(storageinv.Source)
	if !ok {
		writeError(w, http.StatusNotImplemented, "docker volume management is not configured on this control plane")
		return
	}
	rawVolumes, err := src.ListAllVolumes(r.Context())
	if err != nil {
		rt.internalError(w, "api: storage inventory: list volumes failed", err)
		return
	}
	rawContainers, err := src.ListStoppedContainers(r.Context())
	if err != nil {
		rt.internalError(w, "api: storage inventory: list containers failed", err)
		return
	}
	owners, err := rt.storageOwners(r.Context())
	if err != nil {
		rt.internalError(w, "api: storage inventory: resolve owners failed", err)
		return
	}
	writeJSON(w, http.StatusOK, storageInventoryResponse{
		Node:       localNodeID,
		Volumes:    storageinv.BuildVolumes(rawVolumes, owners),
		Containers: storageinv.BuildContainers(rawContainers, owners),
	})
}
