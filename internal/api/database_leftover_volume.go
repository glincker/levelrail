package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dbupgrade"
	"github.com/GLINCKER/levelrail/internal/imageupdate"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	existingVolumeReuse   = "reuse"
	existingVolumeDiscard = "discard"
)

// databaseVolumeNames mirrors internal/reconcile/database's volume naming,
// the same duplication desiredVolumeNames documents.
func databaseVolumeNames(dbName string) []string {
	return []string{"db-" + dbName + "-data", "db-" + dbName + "-certs", "db-" + dbName + "-wal-archive"}
}

// validExistingVolume accepts the stored leftover-volume choice. The volume
// itself is inspected by the reconciler, never by the create request.
func validExistingVolume(choice string) (string, bool) {
	c := strings.TrimSpace(choice)
	switch c {
	case "", existingVolumeReuse, existingVolumeDiscard:
		return c, true
	}
	return c, false
}

const errCodeLeftoverVolumeIncompatible = "leftover_volume_incompatible"

// leftoverDataMajor returns the on-disk major of a leftover data volume for
// name, or "" when there is none, the node is remote, or Docker cannot say.
func (rt *Router) leftoverDataMajor(ctx context.Context, name, engine, nodeID string) (volume, major string) {
	rd, ok := rt.orphanedVolumes.(database.VolumeFileReader)
	if rt.orphanedVolumes == nil || !ok || !rt.isLocalNode(nodeID) {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(ctx, leftoverVolumeTimeout())
	defer cancel()
	vols, err := rt.orphanedVolumes.ListNamedVolumes(ctx)
	if err != nil {
		return "", ""
	}
	vol := databaseVolumeNames(name)[0]
	for _, v := range vols {
		if v.Name == vol {
			return vol, database.DetectVolumeMajor(ctx, rd, engine, vol)
		}
	}
	return "", ""
}

// refuseIncompatibleLeftover writes the 409 and returns true when a leftover
// volume of the same name cannot be opened by the requested version.
func (rt *Router) refuseIncompatibleLeftover(w http.ResponseWriter, r *http.Request, req databaseResource) bool {
	if req.ExistingVolume == existingVolumeDiscard {
		return false
	}
	vol, major := rt.leftoverDataMajor(r.Context(), req.Name, req.Engine, req.NodeID)
	if major == "" || database.VolumeCompatible(req.Engine, major, req.Version) {
		return false
	}
	writeCodedError(w, http.StatusConflict, errCodeLeftoverVolumeIncompatible, database.IncompatibleVolumeMessage(vol, major, req.Version))
	return true
}

// matchLeftoverMajor swaps an auto-resolved version for the newest patch of the
// leftover volume's major, so the default create can open its own data.
func (rt *Router) matchLeftoverMajor(ctx context.Context, req *databaseResource) {
	if req.ExistingVolume == existingVolumeDiscard || req.VersionResolution == nil {
		return
	}
	_, major := rt.leftoverDataMajor(ctx, req.Name, req.Engine, req.NodeID)
	if major == "" || database.VolumeCompatible(req.Engine, major, req.Version) {
		return
	}
	if req.Engine != store.EnginePostgres {
		return
	}
	var inner imageupdate.TagLister = imageupdate.HTTPTagLister{}
	if rt.imageTagLister != nil {
		inner = rt.imageTagLister
	}
	repo := strings.TrimSuffix(database.ImageRef(req.Engine, "x"), ":x")
	if v, ok := dbupgrade.ResolveForMajor(ctx, anonymousTagLister{inner}, repo, major); ok {
		req.Version = v
		req.VersionResolution.Version = v
		req.VersionResolution.Source = "leftover_volume"
	}
}
