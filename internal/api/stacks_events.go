package api

import (
	"context"
	"sort"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Stack event types for the event webhooks.
const (
	stackEventApplied           = "stacks.applied"
	stackEventApprovalRequested = "stacks.approval_requested"
	stackEventApproved          = "stacks.approved"
	stackEventRolledBack        = "stacks.rolled_back"
	stackEventDrifted           = "stacks.drifted"
	stackEventApplyFailed       = "stacks.apply_failed"

	// stackEventHashPrefix is enough to tell plans apart without echoing the full hash.
	stackEventHashPrefix = 12
)

// stackEventPayload holds keys and counts only: item names, counts, a plan hash
// prefix and the actor. A value of any resource field never enters it.
func stackEventPayload(stack, actor, planHash string, revision int, names []string, counts map[string]int, maxNames int) map[string]any {
	items := append([]string{}, names...)
	sort.Strings(items)
	truncated := false
	if maxNames > 0 && len(items) > maxNames {
		items, truncated = items[:maxNames], true
	}
	if len(planHash) > stackEventHashPrefix {
		planHash = planHash[:stackEventHashPrefix]
	}
	out := map[string]any{"stack": stack, "actor": actor, "items": items, "items_truncated": truncated}
	if revision > 0 {
		out["revision"] = revision
	}
	if planHash != "" {
		out["plan_hash"] = planHash
	}
	if len(counts) > 0 {
		c := make(map[string]int, len(counts))
		for k, v := range counts {
			c[k] = v
		}
		out["counts"] = c
	}
	return out
}

// emitStackEvent queues one stack event; it is a no-op without event webhooks.
func (rt *Router) emitStackEvent(ctx context.Context, typ, stack, actor, planHash string, revision int, names []string, counts map[string]int) {
	if rt.eventWebhooks == nil {
		return
	}
	maxNames := 0
	if rt.runtimeSettings != nil {
		maxNames = rt.runtimeSettings.Int(ctx, runtimesettings.KeyStackDriftSummaryMax)
	}
	rt.eventWebhooks.Emit(eventhook.Event{Type: typ, Data: stackEventPayload(stack, actor, planHash, revision, names, counts, maxNames)})
}
