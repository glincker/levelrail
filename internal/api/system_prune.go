package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// systemPruneResponse is POST /api/v1/system/prune's wire shape: exactly
// what got removed and how much space came back, per resource kind, plus
// any per-stage error that didn't stop the rest (docker.PruneResult's own
// doc comment). Mirrors drainNodeResponse's own "report what happened,
// don't just say done" shape (internal/api/nodes.go) for the same reason:
// this is a fleet-wide, no-undo action, so an operator needs to see
// exactly what it did.
type systemPruneResponse struct {
	ContainersRemoved        []string `json:"containers_removed"`
	ContainersReclaimedBytes uint64   `json:"containers_reclaimed_bytes"`
	ImagesRemoved            []string `json:"images_removed"`
	ImagesReclaimedBytes     uint64   `json:"images_reclaimed_bytes"`
	VolumesRemoved           []string `json:"volumes_removed"`
	VolumesReclaimedBytes    uint64   `json:"volumes_reclaimed_bytes"`
	BuildCacheReclaimedBytes uint64   `json:"build_cache_reclaimed_bytes"`
	Errors                   []string `json:"errors,omitempty"`
}

// handleSystemPrune handles POST /api/v1/system/prune: a manual "clean
// up now" action (the same one-click-action shape RestartAppButton's own
// POST /apps/{name}/restart already establishes on the frontend), gated
// at AbilityRoot (router.go's own route registration) because unlike a
// single app's restart, this touches every stopped container, dangling
// image, and anonymous volume on the whole daemon, not one app's own
// resources.
//
// 501 when no DockerPruner is configured (this control plane started
// without a working Docker connection), the same "not configured" shape
// every other optional-dependency route in this package already uses
// (see Builder's own 501 branch on handleTriggerBuild). 500 only if this
// handler's own desired-state lookup fails before Prune is even called;
// once Prune runs, every stage's own failure is reported inside the 200
// response body (Errors), never surfaced as a 5xx, matching
// handleDrainNode's own "partial failure is still a real, reportable
// outcome" shape.
func (rt *Router) handleSystemPrune(w http.ResponseWriter, r *http.Request) {
	if rt.dockerPruner == nil {
		writeError(w, http.StatusNotImplemented, "docker pruning is not configured on this control plane")
		return
	}

	keep, err := rt.desiredContainerNames(r.Context())
	if err != nil {
		rt.logger.Error("api: system prune: compute desired container names failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result := rt.dockerPruner.Prune(r.Context(), keep)
	if len(result.Errors) > 0 {
		rt.logger.Error("api: system prune: one or more stages failed",
			slog.Any("errors", result.Errors),
			slog.Int("containers_removed", len(result.ContainersRemoved)),
			slog.Int("images_removed", len(result.ImagesRemoved)),
			slog.Int("volumes_removed", len(result.VolumesRemoved)),
		)
	}

	writeJSON(w, http.StatusOK, systemPruneResponse{
		ContainersRemoved:        orEmpty(result.ContainersRemoved),
		ContainersReclaimedBytes: result.ContainersReclaimedBytes,
		ImagesRemoved:            orEmpty(result.ImagesRemoved),
		ImagesReclaimedBytes:     result.ImagesReclaimedBytes,
		VolumesRemoved:           orEmpty(result.VolumesRemoved),
		VolumesReclaimedBytes:    result.VolumesReclaimedBytes,
		BuildCacheReclaimedBytes: result.BuildCacheReclaimedBytes,
		Errors:                   result.Errors,
	})
}

// orEmpty turns a nil slice into an empty one so the JSON wire shape is
// always "[]", never "null", the same convention handleListImages
// already follows for its own []imageResource return.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// desiredContainerNames returns every container name the reconciler
// currently considers desired: every replica of every application
// service, plus one per database. This is docker.Client.PruneContainers'
// keep list (internal/docker/prune.go), so a stopped-but-desired
// container (application.Controller's ensureReplicaRunning restarts a
// crashed container in place by ID rather than always recreating one)
// is never treated as garbage by POST /system/prune.
//
// The database container name formula ("db-" + name) is duplicated here
// from internal/reconcile/database's own unexported containerName
// rather than imported: that package's reconciler-core convergence
// logic is deliberately out of scope to modify for this feature (unlike
// internal/reconcile/application.ContainerName, which was already
// exported for exactly this cross-package "derive the same name"
// purpose before this feature existed, see internal/reconcile/ingress's
// own use of it). If internal/reconcile/database's naming ever changes,
// this must change with it; there is no compiler-enforced link between
// the two, only this comment.
func (rt *Router) desiredContainerNames(ctx context.Context) ([]string, error) {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list desired services: %w", err)
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list desired databases: %w", err)
	}

	names := make([]string, 0, len(services)+len(databases))
	for _, svc := range services {
		names = append(names, desiredServiceContainerNames(svc)...)
	}
	for _, db := range databases {
		names = append(names, "db-"+db.Name)
	}
	return names, nil
}

// desiredServiceContainerNames is every replica target name svc's
// current desired state could produce, mirroring
// internal/reconcile/application's own replicaContainerName (unexported
// there, reproduced here rather than exported purely for this one extra
// caller: it is a one-line formula built directly on top of the already-
// exported application.ContainerName, not independent logic that could
// drift the way a second hashing scheme would).
func desiredServiceContainerNames(svc store.DesiredService) []string {
	replicas := svc.Replicas
	if replicas < 1 {
		replicas = 1
	}
	base := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)
	names := make([]string, 0, replicas)
	names = append(names, base)
	for i := 1; i < replicas; i++ {
		names = append(names, base+"-r"+strconv.Itoa(i))
	}
	return names
}
