package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Kinds and statuses a node resource row reports.
const (
	nodeResourceKindApp      = "app"
	nodeResourceKindDatabase = "database"
	nodeResourceActive       = "active"
	nodeResourceSuspended    = "suspended"
)

type nodeResourceItem struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Detail      string `json:"detail"`
	Status      string `json:"status"`
	Replicas    int    `json:"replicas,omitempty"`
	MemoryBytes int64  `json:"memory_bytes,omitempty"`
	NanoCPUs    int64  `json:"nano_cpus,omitempty"`
}

type nodeResourcesResponse struct {
	NodeID    string             `json:"node_id"`
	Resources []nodeResourceItem `json:"resources"`
}

func nodeResourceStatus(suspended bool) string {
	if suspended {
		return nodeResourceSuspended
	}
	return nodeResourceActive
}

func nodeResourceSize(res *store.ServiceResources) (int64, int64) {
	if res == nil {
		return 0, 0
	}
	return res.MemoryBytes, res.NanoCPUs
}

// handleListNodeResources handles GET /api/v1/nodes/{id}/resources: the apps
// and databases placed on a node, filtered to what the caller may read.
func (rt *Router) handleListNodeResources(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: node resources: look up node failed", err, slog.String("node_id", id))
		return
	}
	canReadApp, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: node resources: caller scope failed", err, slog.String("node_id", id))
		return
	}
	canReadDB, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: node resources: caller scope failed", err, slog.String("node_id", id))
		return
	}
	// Workloads on the control plane's own node are stored with node_id "".
	ids := []string{id}
	if rt.localNodeID != "" && id == rt.localNodeID {
		ids = append(ids, "")
	}
	var services []store.DesiredService
	var databases []store.DesiredDatabase
	for _, nid := range ids {
		svcs, err := rt.apps.ListDesiredServicesByNode(r.Context(), nid)
		if err != nil {
			rt.internalError(w, "api: node resources: list services failed", err, slog.String("node_id", id))
			return
		}
		dbs, err := rt.databases.ListDesiredDatabasesByNode(r.Context(), nid)
		if err != nil {
			rt.internalError(w, "api: node resources: list databases failed", err, slog.String("node_id", id))
			return
		}
		services, databases = append(services, svcs...), append(databases, dbs...)
	}
	out := nodeResourcesResponse{NodeID: id, Resources: make([]nodeResourceItem, 0, len(services)+len(databases))}
	for _, s := range services {
		if !canReadApp(s.Name) {
			continue
		}
		mem, cpu := nodeResourceSize(s.Resources)
		out.Resources = append(out.Resources, nodeResourceItem{
			Kind: nodeResourceKindApp, Name: s.Name, Detail: s.Image, Status: nodeResourceStatus(s.Suspended),
			Replicas: s.Replicas, MemoryBytes: mem, NanoCPUs: cpu,
		})
	}
	for _, d := range databases {
		if !canReadDB(d.Name) {
			continue
		}
		mem, cpu := nodeResourceSize(d.Resources)
		out.Resources = append(out.Resources, nodeResourceItem{
			Kind: nodeResourceKindDatabase, Name: d.Name, Detail: d.Engine + " " + d.Version, Status: nodeResourceStatus(d.Suspended),
			MemoryBytes: mem, NanoCPUs: cpu,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
