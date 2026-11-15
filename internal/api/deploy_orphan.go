package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// cancelOrphanedDeploy finishes a running attempt that no build in this process
// owns, once it is older than the orphan grace. It writes its own conflict
// response and reports whether the attempt was finalized.
func (rt *Router) cancelOrphanedDeploy(w http.ResponseWriter, r *http.Request, a *store.DeployAttempt, by string) bool {
	ctx := r.Context()
	grace := time.Duration(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDeployOrphanGraceSeconds)) * time.Second
	rt.buildStartMu.Lock()
	starting := rt.startingDeploys[a.ServiceName] > 0
	rt.buildStartMu.Unlock()
	if starting || time.Since(a.StartedAt) < grace {
		rt.writeCancelConflict(w, r, a.ID, "this deploy is not in progress on this control plane; if it only just started, retry in "+strconv.Itoa(int(grace.Seconds()))+" seconds")
		return false
	}
	moved, err := rt.deploySafety.CancelOrphanedDeployAttempt(ctx, a.ID, by, time.Now())
	if err != nil {
		rt.internalError(w, "api: cancel orphaned deploy failed", err, slog.String("attempt_id", a.ID))
		return false
	}
	if !moved {
		rt.writeCancelConflict(w, r, a.ID, "the deploy finished before the cancel landed")
		return false
	}
	return true
}

// interruptedDeployAttentionItems lists apps whose latest deploy a restart cut short.
func (rt *Router) interruptedDeployAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if rt.deployAttempts == nil || !hasAbility(abilities, AbilityDeploy) {
		return nil
	}
	failed, err := rt.deployAttempts.ListFailedDeploysSince(r.Context(), time.Time{})
	if err != nil {
		rt.logger.Warn("api: attention: list failed deploys failed", slog.String("error", err.Error()))
		return nil
	}
	var out []attention.Item
	for _, f := range failed {
		if f.Attempt.Status == store.DeployAttemptStatusRefused {
			it := feedItem(attention.Warning, attention.KindDeployRefused, f.Attempt.ServiceName,
				"Refused: "+f.Attempt.Error+". Free space on the node or open Nodes, Capacity, then retry",
				map[string]string{"app": f.Attempt.ServiceName, "deploy": f.Attempt.ID})
			it.Title = "Deploy of " + f.Attempt.ServiceName + " was refused"
			out = append(out, it)
			continue
		}
		if f.Attempt.Reason != store.DeployReasonInterrupted {
			continue
		}
		it := feedItem(attention.Warning, attention.KindDeployInterrupted, f.Attempt.ServiceName,
			store.DeployReasonInterrupted+"; deploy it again to retry", map[string]string{"app": f.Attempt.ServiceName, "deploy": f.Attempt.ID})
		it.Title = "Deploy of " + f.Attempt.ServiceName + " was interrupted"
		out = append(out, it)
	}
	return out
}
