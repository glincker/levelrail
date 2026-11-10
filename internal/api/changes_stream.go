package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// changeMessage is one SSE message on GET /api/v1/events/changes: topic
// names only, so a client refetches through its own authorized reads.
type changeMessage struct {
	Topics []string `json:"topics"`
}

func changesStreamResource(*http.Request) string { return "events" }

// changeFilter decides which nudges a caller may see, from the same scope
// the list endpoints use, so a team or org never learns of another's writes.
type changeFilter struct {
	abilities []string
	policies  []store.Policy
	scope     *resourceScope
}

func (rt *Router) changeFilterFor(r *http.Request) (changeFilter, error) {
	abilities, policies, scope, err := rt.callerScope(r)
	if err != nil {
		return changeFilter{}, err
	}
	return changeFilter{abilities: abilities, policies: policies, scope: scope}, nil
}

func (f changeFilter) allows(ev changeEvent) bool {
	if ev.topic == changeNodes {
		return f.allowsTopic(changeNodes)
	}
	if f.ownerUnknown(ev) {
		// A name with no owner row (just created elsewhere, or deleted) is only safe for an all teams caller.
		return f.scope.teams.allTeams
	}
	return f.scope.authorize(f.abilities, f.policies, AbilityRead, resourcePrefixApp, ev.name)
}

// ownerUnknown is true when team scoping applies but the owner map loaded
// at the last refresh has no row for the app.
func (f changeFilter) ownerUnknown(ev changeEvent) bool {
	return ev.topic != changeNodes && f.scope.teams != nil && f.scope.appOwner[ev.name] == nil
}

// allowsTopic is used for coalesced overflow, where the resource name is
// gone, so only callers that can read every resource of the topic qualify.
func (f changeFilter) allowsTopic(t changeTopic) bool {
	if f.scope.restricted() || len(f.policies) > 0 {
		return false
	}
	if t == changeNodes {
		return hasAbility(f.abilities, AbilityRoot)
	}
	return hasAbility(f.abilities, AbilityRead)
}

func (rt *Router) changesSettings() *runtimesettings.Service {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings
	}
	return runtimesettings.New(nil, func(string) (string, bool) { return "", false })
}

const (
	changesHeartbeatFloor = 5 * time.Second
	changesHeartbeatCeil  = 5 * time.Minute
)

// changesHeartbeat clamps the setting because env seeds are parsed but not range checked.
func (rt *Router) changesHeartbeat(ctx context.Context) time.Duration {
	return clampHeartbeat(rt.changesSettings().Duration(ctx, runtimesettings.KeyChangesStreamHeartbeat))
}

func clampHeartbeat(d time.Duration) time.Duration {
	switch {
	case d <= 0:
		return 20 * time.Second
	case d < changesHeartbeatFloor:
		return changesHeartbeatFloor
	case d > changesHeartbeatCeil:
		return changesHeartbeatCeil
	}
	return d
}

// changePing tells the client the heartbeat so it can detect a silent, dead stream.
type changePing struct {
	HeartbeatMS int64 `json:"heartbeat_ms"`
}

// handleChangesStream handles GET /api/v1/events/changes: SSE nudges naming
// the topics (apps, nodes, alerts, domains) that changed for this caller.
func (rt *Router) handleChangesStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principalType, principalID, _, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: change stream: resolve caller", err)
		return
	}
	filter, err := rt.changeFilterFor(r)
	if err != nil {
		rt.internalError(w, "api: change stream: resolve scope", err)
		return
	}
	settings := rt.changesSettings()
	sub, ok := rt.changes.subscribe(principalType+":"+principalID,
		settings.Limit(ctx, runtimesettings.KeyChangesStreamBuffer),
		settings.Limit(ctx, runtimesettings.KeyChangesStreamMaxPerCaller))
	if !ok {
		writeError(w, http.StatusTooManyRequests, "too many open change streams for this caller")
		return
	}
	defer rt.changes.unsubscribe(sub)

	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := rt.startLiveSSE(w, r)
	if !ok {
		return
	}
	defer footprint.Attribute("changes-stream")()
	rc := http.NewResponseController(w)
	beat := rt.changesHeartbeat(ctx)
	// A write deadline per frame frees the slot of a peer that stopped reading.
	send := func(frame string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(2 * beat))
		if _, err := fmt.Fprint(w, frame); err != nil { //nolint:gosec // frames are SSE text built here from constants and JSON, never request input
			return false
		}
		flusher.Flush()
		return true
	}
	ping := func() bool {
		data, _ := json.Marshal(changePing{HeartbeatMS: beat.Milliseconds()})
		return send("event: ping\ndata: " + string(data) + "\n\n")
	}
	emit := func(mask uint32) bool {
		if mask == 0 {
			return true
		}
		msg := changeMessage{}
		for t := changeTopic(0); t < changeTopicCount; t++ {
			if mask&(1<<t) != 0 {
				msg.Topics = append(msg.Topics, changeTopicNames[t])
			}
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return true
		}
		return send("data: " + string(data) + "\n\n")
	}
	refresh := func() changeFilter {
		if fresh, err := rt.changeFilterFor(r); err == nil {
			filter = fresh
		}
		return filter
	}
	if !send(": connected\n\n") || !ping() {
		return
	}

	ticker := time.NewTicker(beat)
	defer ticker.Stop()
	for {
		ok := true
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Access is re-read here and the reauth watcher ends the stream on loss.
			refresh()
			if next := rt.changesHeartbeat(ctx); next != beat {
				beat = next
				ticker.Reset(beat)
			}
			ok = emit(coalesceChanges(sub, filter, nil, refresh)) && ping()
		case <-sub.wake:
			ok = emit(coalesceChanges(sub, filter, nil, refresh))
		case ev := <-sub.ch:
			ok = emit(coalesceChanges(sub, filter, &ev, refresh))
		}
		if !ok {
			return
		}
	}
}

// coalesceChanges folds first, whatever is already queued (bounded by the
// buffer size) and any overflow into one topic bitmask. The filter is
// refreshed at most once when an app has no owner row yet.
func coalesceChanges(sub *changeSub, f changeFilter, first *changeEvent, refresh func() changeFilter) uint32 {
	var mask uint32
	refreshed := false
	take := func(ev changeEvent) {
		if f.ownerUnknown(ev) && !refreshed && refresh != nil {
			f, refreshed = refresh(), true
		}
		if f.allows(ev) {
			mask |= 1 << ev.topic
		}
	}
	if first != nil {
		take(*first)
	}
	for i := 0; i < cap(sub.ch); i++ {
		select {
		case ev := <-sub.ch:
			take(ev)
			continue
		default:
		}
		break
	}
	if dropped := sub.overflow.Swap(0); dropped != 0 {
		for t := changeTopic(0); t < changeTopicCount; t++ {
			if dropped&(1<<t) != 0 && f.allowsTopic(t) {
				mask |= 1 << t
			}
		}
	}
	return mask
}
