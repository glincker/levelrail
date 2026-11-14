package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// stackCloudNow is a test seam for the cloud plan age check.
var stackCloudNow = time.Now

// planCloudChanges counts the cloud items a plan would create, update or delete.
func planCloudChanges(plan iac.Plan) int {
	n := 0
	for _, c := range plan.Changes {
		if iac.IsCloudKind(c.Kind) && (c.Action == iac.ActionCreate || c.Action == iac.ActionUpdate || c.Action == iac.ActionDelete) {
			n++
		}
	}
	return n
}

// revisionCloudPlan reads the plan stored with a revision; ok is false when it holds no cloud item.
func revisionCloudPlan(rev store.StackRevisionRecord) (iac.Plan, bool) {
	var plan iac.Plan
	if len(rev.PlanJSON) == 0 || json.Unmarshal(rev.PlanJSON, &plan) != nil {
		return plan, false
	}
	for _, c := range plan.Changes {
		if iac.IsCloudKind(c.Kind) {
			return plan, true
		}
	}
	return plan, false
}

// cloudApprovalOf pins the cloud items of the approved plan, so the apply runs the
// saved engine plans the approver reviewed and never plans afresh.
func cloudApprovalOf(plan iac.Plan, confirmDestroy bool) *iac.CloudApproval {
	ap := &iac.CloudApproval{Pins: map[string]iac.Change{}, ConfirmDestroy: confirmDestroy}
	for _, c := range plan.Changes {
		if iac.IsCloudKind(c.Kind) {
			ap.Pins[c.Key()] = c
		}
	}
	return ap
}

// refuseUnapprovedCloud refuses a cloud change outside an approved revision, whatever
// the stack or solo settings say.
func refuseUnapprovedCloud(w http.ResponseWriter, run stackRun, plan iac.Plan) bool {
	if run.pending != nil || planCloudChanges(plan) == 0 {
		return false
	}
	writeIaCError(w, http.StatusForbidden, iac.CodeApprovalRequired,
		"cloud changes apply only from an approved revision; turn on require_approval for the stack and have another administrator approve the plan", nil)
	return true
}

// cloudApprovalTooOld refuses an approval whose saved engine plans are past cloud.plan_max_age_minutes.
func (rt *Router) cloudApprovalTooOld(ctx context.Context, w http.ResponseWriter, rev store.StackRevisionRecord) bool {
	maxAge := time.Duration(0)
	if rt.runtimeSettings != nil {
		maxAge = time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyCloudPlanMaxAgeMinutes)) * time.Minute
	}
	if maxAge > 0 && stackCloudNow().Sub(rev.CreatedAt) <= maxAge {
		return false
	}
	writeIaCError(w, http.StatusGone, iac.CodeCloudPlanExpired,
		fmt.Sprintf("the cloud plan of revision %d is older than cloud.plan_max_age_minutes; request a new apply", rev.Number), nil)
	return true
}

// emitCloudDiverged reports a quarantined engine run as apply_failed with the quarantine id, never state.
func (rt *Router) emitCloudDiverged(ctx context.Context, stack, actor string, revision int, runs []iac.CloudRun) bool {
	sent := false
	for _, r := range runs {
		if !r.Diverged {
			continue
		}
		sent = true
		if rt.eventWebhooks == nil {
			continue
		}
		maxNames := 0
		if rt.runtimeSettings != nil {
			maxNames = rt.runtimeSettings.Int(ctx, runtimesettings.KeyStackDriftSummaryMax)
		}
		rt.eventWebhooks.Emit(eventhook.Event{Type: stackEventApplyFailed, Data: cloudDivergedPayload(stack, actor, revision, r, maxNames)})
	}
	return sent
}

func cloudDivergedPayload(stack, actor string, revision int, r iac.CloudRun, maxNames int) map[string]any {
	out := stackEventPayload(stack, actor, r.PlanHash, revision, r.Items, map[string]int{"failed": len(r.Items)}, maxNames)
	out["reason"] = iac.CodeStateDiverged
	out["quarantine_id"] = r.QuarantineID
	return out
}

// resourcesHaveCloud reports whether any document is a cloud kind, so a first apply can be refused before any row exists.
func resourcesHaveCloud(res []*iac.Resource) bool {
	for _, r := range res {
		if iac.IsCloudKind(r.Kind) {
			return true
		}
	}
	return false
}
