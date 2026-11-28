package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/glincker/levelrail/internal/dbprovision"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/store"
)

const (
	errCodeLeftoverOtherTeam    = "leftover_volume_other_team"
	errCodeLeftoverOwnerUnknown = "leftover_volume_owner_unknown"
	errCodeLeftoverConfirm      = "leftover_volume_confirm_required"

	errCodeLeftoverNodeUnreachable = "leftover_volume_node_unreachable"
	msgLeftoverNodeUnreachable     = "the node cannot be asked whether an earlier database left data under this name (agent offline or too old); bring the agent online or upgrade it, then retry"

	msgLeftoverOtherTeam    = "a volume with this name from another team exists; choose discard or another name"
	msgLeftoverOwnerUnknown = "a volume with this name of unknown ownership exists; choose discard or another name"
	msgLeftoverConfirm      = "reusing a volume of unknown ownership needs confirm_volume set to the database name"

	auditLeftoverDiscard = "database.volume.discard"
	auditLeftoverReuse   = "database.volume.reuse_unowned"
)

// databaseVolumeOwnerStore is the owner table the real store implements.
type databaseVolumeOwnerStore interface {
	GetDatabaseVolumeOwner(ctx context.Context, name string) (*store.DatabaseVolumeOwner, error)
	SetDatabaseVolumeOwner(ctx context.Context, o store.DatabaseVolumeOwner) error
	DeleteDatabaseVolumeOwner(ctx context.Context, name string) error
}

func (rt *Router) volumeOwners() databaseVolumeOwnerStore {
	s, _ := rt.databases.(databaseVolumeOwnerStore)
	return s
}

// volumeOwnerLocks serializes owner decisions per database name; it is separate
// from the app name lock so a create holding that one cannot deadlock here.
var volumeOwnerLocks = struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}{m: map[string]*sync.Mutex{}}

func lockVolumeOwner(name string) func() {
	volumeOwnerLocks.mu.Lock()
	l := volumeOwnerLocks.m[name]
	if l == nil {
		l = &sync.Mutex{}
		volumeOwnerLocks.m[name] = l
	}
	volumeOwnerLocks.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// teamOfProjectID is the team that owns a new database in projectID.
func (rt *Router) teamOfProjectID(ctx context.Context, projectID string) string {
	db := rt.teamDB()
	if db == nil || projectID == "" {
		return store.DefaultTeamID
	}
	team, err := db.TeamOfProject(ctx, projectID)
	if err != nil || team == "" {
		return store.DefaultTeamID
	}
	return team
}

// teamOfDatabase is the team of an existing database record.
func (rt *Router) teamOfDatabase(ctx context.Context, name string) (team, project string) {
	db := rt.teamDB()
	if db == nil {
		return store.DefaultTeamID, ""
	}
	o, err := db.OwnerOfDatabase(ctx, name)
	if err != nil || o == nil {
		return store.DefaultTeamID, ""
	}
	return o.TeamID, o.ProjectID
}

// nodeVolumeLister is the agent transport's volume listing, absent on older agents.
type nodeVolumeLister interface {
	ListNodeVolumes(ctx context.Context) ([]docker.NamedVolume, error)
}

// remoteVolumeGate is true when a database on nodeID must be checked through its agent.
func (rt *Router) remoteVolumeGate(nodeID string) bool {
	return rt.orphanedVolumes != nil && !rt.isLocalNode(nodeID)
}

// listNodeVolumes lists named volumes on the node: Docker for the control plane,
// the agent for a remote node. An agent that cannot answer is an error, never an empty list.
func (rt *Router) listNodeVolumes(ctx context.Context, nodeID string) ([]docker.NamedVolume, error) {
	if rt.isLocalNode(nodeID) {
		return rt.orphanedVolumes.ListNamedVolumes(ctx)
	}
	runtime, err := rt.execRuntimeFor(nodeID)
	if err != nil {
		return nil, err
	}
	l, ok := runtime.(nodeVolumeLister)
	if !ok {
		return nil, errors.New("node cannot list volumes")
	}
	return l.ListNodeVolumes(ctx)
}

// removeNodeVolume deletes one unmounted volume on the node the database lives on.
func (rt *Router) removeNodeVolume(ctx context.Context, nodeID, vol string) error {
	if rt.isLocalNode(nodeID) {
		return rt.orphanedVolumes.RemoveVolume(ctx, vol)
	}
	runtime, err := rt.execRuntimeFor(nodeID)
	if err != nil {
		return err
	}
	rm, ok := runtime.(docker.VolumeRemover)
	if !ok {
		return errors.New("node cannot remove volumes")
	}
	return rm.RemoveVolume(ctx, vol)
}

// leftoverVolumeState reports whether the data volume exists on the database's
// node; known is false when the node cannot be asked.
func (rt *Router) leftoverVolumeState(ctx context.Context, name, nodeID string) (present, mounted, known bool) {
	if rt.orphanedVolumes == nil {
		return false, false, false
	}
	ctx, cancel := context.WithTimeout(ctx, leftoverVolumeTimeout())
	defer cancel()
	vols, err := rt.listNodeVolumes(ctx, nodeID)
	if err != nil {
		return false, false, false
	}
	vol := databaseVolumeNames(name)[0]
	for _, v := range vols {
		if v.Name == vol {
			return true, v.Mounted, true
		}
	}
	return false, false, true
}

type volumeVerdict int

const (
	volVerdictAllowed volumeVerdict = iota
	volVerdictOtherTeam
	volVerdictUnknown
)

func (rt *Router) volumeOwnerVerdict(ctx context.Context, name, team string) volumeVerdict {
	vs := rt.volumeOwners()
	if vs == nil {
		return volVerdictAllowed
	}
	rec, err := vs.GetDatabaseVolumeOwner(ctx, name)
	switch {
	case err != nil || rec == nil:
		return volVerdictUnknown
	case rec.TeamID != team:
		return volVerdictOtherTeam
	}
	return volVerdictAllowed
}

// leftoverChoiceAllowed gates a reuse choice on volume ownership and writes the
// refusal. Discard and no choice always pass: they never expose the old data.
func (rt *Router) leftoverChoiceAllowed(w http.ResponseWriter, r *http.Request, name, nodeID, team, choice, confirm string) bool {
	if rt.volumeOwners() == nil {
		return true
	}
	remote := rt.remoteVolumeGate(nodeID)
	present, _, known := rt.leftoverVolumeState(r.Context(), name, nodeID)
	if remote {
		if !known {
			// Only the recorded owner may proceed blind: anyone else could mount old data.
			if rt.volumeOwnerVerdict(r.Context(), name, team) == volVerdictAllowed {
				return true
			}
			writeCodedError(w, http.StatusConflict, errCodeLeftoverNodeUnreachable, msgLeftoverNodeUnreachable)
			return false
		}
		if !present || choice == existingVolumeDiscard {
			return true
		}
	} else if choice != existingVolumeReuse || (known && !present) {
		return true
	}
	switch rt.volumeOwnerVerdict(r.Context(), name, team) {
	case volVerdictAllowed:
		return true
	case volVerdictOtherTeam:
		writeCodedError(w, http.StatusForbidden, errCodeLeftoverOtherTeam, msgLeftoverOtherTeam)
		return false
	}
	if !rt.callerIsRoot(r) {
		writeCodedError(w, http.StatusForbidden, errCodeLeftoverOwnerUnknown, msgLeftoverOwnerUnknown)
		return false
	}
	if confirm != name {
		writeCodedError(w, http.StatusConflict, errCodeLeftoverConfirm, msgLeftoverConfirm)
		return false
	}
	rt.auditVolumeOwnerEvent(r, auditLeftoverReuse, name, "owner=unknown")
	return true
}

func (rt *Router) auditVolumeOwnerEvent(r *http.Request, action, name, detail string) {
	if rt.auditLog == nil {
		return
	}
	rt.auditDatabaseAccess(r, action, name, detail, http.StatusOK)
}

// discardLeftoverVolume wipes a kept data volume under the owner lock and
// records who held it. A volume it cannot wipe here is left to the reconciler
// and its owner record is dropped, so nobody can reuse it by name afterwards.
func (rt *Router) discardLeftoverVolume(r *http.Request, name, nodeID, team, project string) {
	vs := rt.volumeOwners()
	if vs == nil {
		return
	}
	unlock := lockVolumeOwner(name)
	defer unlock()
	ctx := r.Context()
	prev, err := vs.GetDatabaseVolumeOwner(ctx, name)
	if err != nil {
		rt.logger.Warn("api: discard leftover volume: read owner failed", slog.String("error", err.Error()), slog.String("database", name))
	}
	present, mounted, known := rt.leftoverVolumeState(ctx, name, nodeID)
	wiped := known && !present
	if known && present && !mounted {
		if rerr := rt.removeNodeVolume(ctx, nodeID, databaseVolumeNames(name)[0]); rerr != nil {
			rt.logger.Warn("api: discard leftover volume failed", slog.String("error", rerr.Error()), slog.String("database", name))
		} else {
			wiped = true
		}
	}
	if present || prev != nil {
		detail := "prev_team=unknown"
		if prev != nil {
			detail = "prev_team=" + prev.TeamID + ";prev_project=" + prev.ProjectID
		}
		rt.auditVolumeOwnerEvent(r, auditLeftoverDiscard, name, detail)
	}
	if !wiped {
		if derr := vs.DeleteDatabaseVolumeOwner(ctx, name); derr != nil {
			rt.logger.Warn("api: discard leftover volume: drop owner failed", slog.String("error", derr.Error()), slog.String("database", name))
		}
		return
	}
	if serr := vs.SetDatabaseVolumeOwner(ctx, store.DatabaseVolumeOwner{Name: name, TeamID: team, ProjectID: project}); serr != nil {
		rt.logger.Warn("api: discard leftover volume: record owner failed", slog.String("error", serr.Error()), slog.String("database", name))
	}
}

// recordDatabaseVolumeOwner stamps the creator's team on the data volume of a
// fresh database. A kept volume keeps its recorded owner until it is discarded.
func (rt *Router) recordDatabaseVolumeOwner(r *http.Request, name, nodeID, projectID, choice string) {
	vs := rt.volumeOwners()
	if vs == nil {
		return
	}
	team := rt.teamOfProjectID(r.Context(), projectID)
	if choice == existingVolumeDiscard {
		rt.discardLeftoverVolume(r, name, nodeID, team, projectID)
		return
	}
	unlock := lockVolumeOwner(name)
	defer unlock()
	ctx := r.Context()
	rec, err := vs.GetDatabaseVolumeOwner(ctx, name)
	if err != nil {
		rt.logger.Warn("api: record volume owner: read failed", slog.String("error", err.Error()), slog.String("database", name))
		return
	}
	present, _, known := rt.leftoverVolumeState(ctx, name, nodeID)
	mine := store.DatabaseVolumeOwner{Name: name, TeamID: team, ProjectID: projectID}
	var werr error
	switch {
	case known && !present, rec == nil && !known:
		werr = vs.SetDatabaseVolumeOwner(ctx, mine)
	case rec != nil && rec.TeamID != team && (!known || choice == existingVolumeReuse):
		// Unverifiable or reuse of an unowned volume: no one inherits it silently.
		werr = vs.DeleteDatabaseVolumeOwner(ctx, name)
	case rec == nil && choice == existingVolumeReuse:
		werr = vs.SetDatabaseVolumeOwner(ctx, mine)
	}
	if werr != nil {
		rt.logger.Warn("api: record volume owner failed", slog.String("error", werr.Error()), slog.String("database", name))
	}
}

// redactLeftoverPrompt keeps the stop prompt generic and drops the reuse
// choice when the caller's team may not reuse the kept volume.
func (rt *Router) redactLeftoverPrompt(ctx context.Context, name string, a *dbprovision.Attempt) {
	if a == nil || a.Failure == nil || a.Failure.Volume == nil || rt.volumeOwners() == nil {
		return
	}
	team, _ := rt.teamOfDatabase(ctx, name)
	v := rt.volumeOwnerVerdict(ctx, name, team)
	if v == volVerdictAllowed {
		return
	}
	a.Failure.Reason = msgLeftoverOtherTeam
	if v == volVerdictUnknown {
		a.Failure.Reason = msgLeftoverOwnerUnknown
	}
	fixes := make([]dbprovision.Fix, 0, len(a.Failure.Fixes))
	for _, f := range a.Failure.Fixes {
		if f.ID != dbprovision.FixReuseVolume {
			fixes = append(fixes, f)
		}
	}
	a.Failure.Fixes = fixes
}
