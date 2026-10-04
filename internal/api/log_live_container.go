package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file resolves which container ID(s) are a resource's
// authoritative current ones right now, the live log filter's input
// (streamResourceLogs, live_logs.go). Reuses the exact
// application.ContainerName formula exec.go's resolveExecContainer
// already uses, rather than inventing a second notion of "current."

// currentContainerLookup resolves resourceID's current container ID set.
// ok false means "can't tell right now," never "nothing is current":
// callers must not filter on an unresolvable result. A true ok with an
// empty ids means "resolvable, and nothing is running."
type currentContainerLookup func(ctx context.Context, name string) (ids []string, ok bool)

// currentAppContainerIDs is the app-kind currentContainerLookup.
func (rt *Router) currentAppContainerIDs(ctx context.Context, name string) ([]string, bool) {
	if rt.execRuntime == nil {
		return nil, false
	}
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return nil, false
	}
	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		return nil, false
	}

	replicas := svc.Replicas
	if replicas <= 0 {
		replicas = store.DefaultReplicas
	}
	image := application.NameImage(*svc)

	var ids []string
	for i := 0; i < replicas; i++ {
		target := application.ReplicaContainerName(svc.Name, image, svc.RestartNonce, i)
		state, err := nodeRuntime.InspectByName(ctx, target)
		if err != nil || state == nil || !state.Running {
			continue
		}
		ids = append(ids, state.ID)
	}
	return ids, true
}

// currentDatabaseContainerIDs is the database-kind currentContainerLookup.
// Databases never run a blue-green overlap (recreate only, one container,
// one deterministic name), so this set is at most one ID, but it still
// goes through the same lookup shape so streamResourceLogs never needs
// to know the difference.
func (rt *Router) currentDatabaseContainerIDs(ctx context.Context, name string) ([]string, bool) {
	if rt.execRuntime == nil {
		return nil, false
	}
	db, err := rt.databases.GetDesiredDatabase(ctx, name)
	if err != nil {
		return nil, false
	}
	nodeRuntime, err := rt.execRuntime(db.NodeID)
	if err != nil {
		return nil, false
	}
	state, err := nodeRuntime.InspectByName(ctx, database.ContainerName(name))
	if err != nil || state == nil || !state.Running {
		return nil, true
	}
	return []string{state.ID}, true
}

// currentContainerFilter is streamResourceLogs' per-connection filtering
// state: which container IDs currently count as "live" for one resource,
// resolved once up front and only re-resolved on demand (see admit)
// rather than polled on a timer, since a line from an unrecognized
// container only ever arrives right around a real deploy cutover.
type currentContainerFilter struct {
	lookup     currentContainerLookup
	name       string
	resolvable bool
	ids        map[string]struct{}
	notCurrent map[string]struct{} // caches a negative resolve so a dying container's remaining lines don't each trigger a fresh lookup
}

func newCurrentContainerFilter(ctx context.Context, lookup currentContainerLookup, name string) *currentContainerFilter {
	f := &currentContainerFilter{lookup: lookup, name: name, notCurrent: map[string]struct{}{}}
	if lookup == nil {
		return f
	}
	ids, ok := lookup(ctx, name)
	f.resolvable = ok
	f.ids = toIDSet(ids)
	return f
}

// admit reports whether a line from containerID belongs in the live
// view, and whether a transition marker should be shown first because
// containerID just became the new current container (a deploy cutover
// happened mid-session). An empty containerID (a row written before
// migrations/0005_log_entries_container_id.sql existed) always passes
// through unfiltered: there's no way to tell, and treating "unknown" as
// "not current" would hide legitimate pre-migration history.
func (f *currentContainerFilter) admit(ctx context.Context, containerID string) (show, transitioned bool) {
	if !f.resolvable || containerID == "" {
		return true, false
	}
	if _, ok := f.ids[containerID]; ok {
		return true, false
	}
	if _, ok := f.notCurrent[containerID]; ok {
		return false, false
	}

	fresh, ok := f.lookup(ctx, f.name)
	if !ok {
		// A transient resolution failure for this one line only: fail
		// closed (don't show an unrecognized container as if it's live)
		// rather than fail open, since showing a line that turns out to
		// be from a dead container is exactly the trust-breaking bug this
		// filter exists to prevent. f.resolvable and f.ids are left
		// alone, so every line already known to be current keeps flowing
		// normally.
		return false, false
	}

	freshSet := toIDSet(fresh)
	if _, nowCurrent := freshSet[containerID]; nowCurrent {
		transitioned = !sameIDSet(f.ids, freshSet)
		f.ids = freshSet
		f.notCurrent = map[string]struct{}{}
		return true, transitioned
	}
	f.notCurrent[containerID] = struct{}{}
	return false, false
}

func toIDSet(ids []string) map[string]struct{} {
	s := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		s[id] = struct{}{}
	}
	return s
}

func sameIDSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}

// previousContainerTransitionLine is the live view's transition marker
// text, sent as sseLogEvent.Line for non-browser consumers (the CLI's
// "apps logs --follow", which prints Stream+Line verbatim); the
// dashboard renders its own translated label for Stream == "system"
// instead (web/src/components/LogRow.tsx), per this repo's i18n rule.
const previousContainerTransitionLine = "previous container instance ended"
