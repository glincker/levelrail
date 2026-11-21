package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applyUpdatePolicy decides whether the agent may apply d on its own and, when
// it may not, records why in d.Waiting and d.Reason. Rollbacks are never gated.
func (rt *Router) applyUpdatePolicy(ctx context.Context, n *store.Node, current string, d *nodeinsight.UpdateDecision, auto, consume bool) {
	if rt.runtimeSettings == nil || d.Action == nodeinsight.UpdateRollback {
		return
	}
	rs := rt.runtimeSettings
	force := slices.Contains(rs.List(ctx, runtimesettings.KeyAgentUpdateForceNodes), n.ID)
	p := nodeinsight.UpdatePolicy{
		Auto: auto, WindowText: rs.String(ctx, runtimesettings.KeyAgentUpdateWindow),
		Channel: rs.String(ctx, runtimesettings.KeyAgentUpdateChannel),
		Held:    slices.Contains(rs.List(ctx, runtimesettings.KeyAgentUpdateHeldNodes), n.ID),
		// Override applies this answer only; the auto setting is never touched.
		Override: force,
	}
	d.Held = p.Held
	apply, why := p.Evaluate(current, d.Target, time.Now())
	d.AutoApply = apply
	if force && consume {
		rt.setNodeList(ctx, runtimesettings.KeyAgentUpdateForceNodes, n.ID, false)
	}
	if !apply {
		d.Waiting = why + "; to update now use Update anyway or: levelrail nodes agent-update apply " + n.ID + " --force"
		d.Reason += " (waiting: " + why + ")"
	}
}

// setNodeList adds or removes one node id in a list setting.
func (rt *Router) setNodeList(ctx context.Context, key, id string, add bool) {
	list := slices.DeleteFunc(slices.Clone(rt.runtimeSettings.List(ctx, key)), func(s string) bool { return s == id })
	if add {
		list = append(list, id)
	}
	raw, err := json.Marshal(list)
	if err == nil {
		_, err = rt.runtimeSettings.Set(ctx, key, raw, "agent-update")
	}
	if err != nil {
		rt.logger.Warn("api: node agent update list save failed", slog.String("key", key), slog.String("error", err.Error()))
	}
}

// handleNodeAgentUpdateControl serves hold, unhold and override for one node.
func (rt *Router) handleNodeAgentUpdateControl(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := rt.nodes.GetNode(r.Context(), id); err != nil {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		if rt.runtimeSettings == nil {
			writeError(w, http.StatusServiceUnavailable, "runtime settings are not available")
			return
		}
		switch action {
		case "hold":
			rt.setNodeList(r.Context(), runtimesettings.KeyAgentUpdateHeldNodes, id, true)
		case "unhold":
			rt.setNodeList(r.Context(), runtimesettings.KeyAgentUpdateHeldNodes, id, false)
		default:
			rt.setNodeList(r.Context(), runtimesettings.KeyAgentUpdateForceNodes, id, true)
		}
		rt.auditNodeAgentUpdate(r, id, action)
		writeJSON(w, http.StatusOK, map[string]any{"node_id": id, "action": action})
	}
}

func (rt *Router) auditNodeAgentUpdate(r *http.Request, nodeID, action string) {
	if rt.auditLog == nil {
		return
	}
	entryID, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	a := rt.accessActor(r)
	entry := store.AuditEntry{
		ID: entryID, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: r.Method, StatusCode: http.StatusOK,
		Path:       fmt.Sprintf("/api/v1/nodes/%s/agent-update/%s", nodeID, action),
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: node agent update audit failed", slog.String("error", err.Error()), slog.String("node_id", nodeID))
	}
}
