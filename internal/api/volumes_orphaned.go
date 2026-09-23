package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// orphanedVolumeResource is one Docker volume GET
// /api/v1/system/volumes/orphaned reports as no longer referenced by
// any app, database, or storage attachment in this control plane's
// current desired state. SizeBytes is nil when the volume driver didn't
// report a size (docker.NamedVolume.SizeBytes == -1), never a fabricated
// 0, so the frontend can distinguish "empty volume" from "unknown size."
type orphanedVolumeResource struct {
	Name      string `json:"name"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func toOrphanedVolumeResource(v docker.NamedVolume) orphanedVolumeResource {
	res := orphanedVolumeResource{Name: v.Name, CreatedAt: v.CreatedAt}
	if v.SizeBytes >= 0 {
		size := v.SizeBytes
		res.SizeBytes = &size
	}
	return res
}

// handleListOrphanedVolumes handles GET
// /api/v1/system/volumes/orphaned: every named Docker volume this
// instance created (docker.Client.ListNamedVolumes) that current
// desired state no longer references (computeOrphanedVolumes below).
// Detection only, no deletion: the same read/destructive-action split
// DockerDiskUsager/DockerPruner already establish for system/status
// versus system/prune. AbilityRead, matching that same read side.
func (rt *Router) handleListOrphanedVolumes(w http.ResponseWriter, r *http.Request) {
	if rt.orphanedVolumes == nil {
		writeError(w, http.StatusNotImplemented, "docker volume management is not configured on this control plane")
		return
	}

	orphans, err := rt.computeOrphanedVolumes(r.Context())
	if err != nil {
		rt.logger.Error("api: list orphaned volumes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]orphanedVolumeResource, 0, len(orphans))
	for _, v := range orphans {
		out = append(out, toOrphanedVolumeResource(v))
	}
	writeJSON(w, http.StatusOK, out)
}

// cleanupOrphanedVolumesRequest is POST
// /api/v1/system/volumes/orphaned/cleanup's request body: the exact set
// of volume names an operator confirmed after reviewing GET
// .../orphaned's own results, never an implicit "delete everything
// currently orphaned." Mirrors the explicit-confirmation shape every
// other destructive UI flow in this codebase uses (e.g.
// DeleteAppDialog), just at the API layer: the frontend is expected to
// show the list, let the operator pick, and send exactly those names
// back.
type cleanupOrphanedVolumesRequest struct {
	Names []string `json:"names"`
}

// cleanupOrphanedVolumesResponse reports exactly what happened per
// requested name, the same "report what happened, don't just say done"
// shape systemPruneResponse already establishes for POST /system/prune.
type cleanupOrphanedVolumesResponse struct {
	Removed        []string `json:"removed"`
	ReclaimedBytes uint64   `json:"reclaimed_bytes"`
	// Skipped holds a requested name that is no longer orphaned by the
	// time cleanup actually ran (e.g. a live app started referencing it
	// again between the operator's own list and confirm), the TOCTOU
	// guard this handler applies by recomputing orphan status itself
	// rather than trusting the client-supplied list blindly.
	Skipped []string `json:"skipped,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// handleCleanupOrphanedVolumes handles POST
// /api/v1/system/volumes/orphaned/cleanup: deletes exactly the volumes
// named in the request body, after re-confirming each one is still
// genuinely orphaned right now (see cleanupOrphanedVolumesResponse's own
// doc comment on Skipped). AbilityRoot, the same fleet-wide, no-undo
// tier POST /system/prune sits behind, not AbilityWrite.
func (rt *Router) handleCleanupOrphanedVolumes(w http.ResponseWriter, r *http.Request) {
	if rt.orphanedVolumes == nil {
		writeError(w, http.StatusNotImplemented, "docker volume management is not configured on this control plane")
		return
	}

	var req cleanupOrphanedVolumesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names is required: select at least one volume to remove")
		return
	}

	orphans, err := rt.computeOrphanedVolumes(r.Context())
	if err != nil {
		rt.logger.Error("api: cleanup orphaned volumes: compute failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	stillOrphaned := make(map[string]docker.NamedVolume, len(orphans))
	for _, v := range orphans {
		stillOrphaned[v.Name] = v
	}

	resp := cleanupOrphanedVolumesResponse{
		Removed: []string{},
		Skipped: []string{},
	}
	for _, name := range req.Names {
		v, ok := stillOrphaned[name]
		if !ok {
			resp.Skipped = append(resp.Skipped, name)
			continue
		}
		if err := rt.orphanedVolumes.RemoveVolume(r.Context(), name); err != nil {
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		resp.Removed = append(resp.Removed, name)
		if v.SizeBytes > 0 {
			resp.ReclaimedBytes += uint64(v.SizeBytes)
		}
	}
	if len(resp.Skipped) == 0 {
		resp.Skipped = nil
	}
	if len(resp.Errors) > 0 {
		rt.logger.Error("api: cleanup orphaned volumes: one or more removals failed", slog.Any("errors", resp.Errors))
	}

	writeJSON(w, http.StatusOK, resp)
}

// computeOrphanedVolumes lists this instance's own named Docker volumes
// (docker.Client.ListNamedVolumes) and filters down to the ones no
// currently-desired app service or database references by name: an app
// or database whose desired state row was deleted leaves its volume
// behind exactly this way, since neither EnsureVolume's own callers nor
// database.Controller.Teardown ever remove a named volume (see those
// functions' own doc comments). A volume still mounted into a container
// is never flagged even if desired state has somehow lost track of it,
// the same second, independent signal PruneAnonymousVolumes' own
// mountedVolumeNames check already provides defense in depth with.
func (rt *Router) computeOrphanedVolumes(ctx context.Context) ([]docker.NamedVolume, error) {
	all, err := rt.orphanedVolumes.ListNamedVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list named volumes: %w", err)
	}
	if len(all) == 0 {
		return nil, nil
	}

	desired, err := rt.desiredVolumeNames(ctx)
	if err != nil {
		return nil, err
	}

	var orphans []docker.NamedVolume
	for _, v := range all {
		if desired[v.Name] || v.Mounted {
			continue
		}
		orphans = append(orphans, v)
	}
	return orphans, nil
}

// desiredVolumeNames is every Docker volume name this control plane's
// current desired state still references: every application service's
// own ServiceVolume.Name (already the resolved Docker volume name,
// store.ServiceVolume's own doc comment), plus every managed database's
// data and certs volume names. The data/certs formulas
// ("db-"+name+"-data", "db-"+name+"-certs") are duplicated here from
// internal/reconcile/database's own unexported dataVolumeName/
// certsVolumeName rather than imported, the same "reconciler-core
// package is out of scope to modify for this feature" reasoning
// desiredContainerNames (system_prune.go) already gives for duplicating
// that package's containerName formula. Including the certs name
// unconditionally, even for a database that never had TLS enabled and so
// never actually got one created, is harmless: it just never matches
// anything ListNamedVolumes returns.
func (rt *Router) desiredVolumeNames(ctx context.Context) (map[string]bool, error) {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list desired services: %w", err)
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list desired databases: %w", err)
	}

	names := make(map[string]bool, len(services)*2+len(databases)*2)
	for _, svc := range services {
		for _, v := range svc.Volumes {
			names[v.Name] = true
		}
	}
	for _, db := range databases {
		names["db-"+db.Name+"-data"] = true
		names["db-"+db.Name+"-certs"] = true
	}
	return names, nil
}
