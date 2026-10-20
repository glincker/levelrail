package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const errCodeRollbackTargetUnknown = "rollback_target_unknown"

func unpinnedRef(ref string) string {
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

// rollbackTargets lists the images a rollback of svc may name: its succeeded
// deploys and pinned releases, newest first, never the image it runs now.
func (rt *Router) rollbackTargets(ctx context.Context, svc store.DesiredService) ([]string, error) {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, svc.Name)
	if err != nil {
		return nil, fmt.Errorf("list deploy attempts: %w", err)
	}
	sort.SliceStable(attempts, func(i, j int) bool { return attempts[i].StartedAt.After(attempts[j].StartedAt) })
	seen := map[string]bool{unpinnedRef(svc.Image): true}
	var out []string
	add := func(ref string) {
		if ref == "" || seen[unpinnedRef(ref)] {
			return
		}
		seen[unpinnedRef(ref)] = true
		out = append(out, ref)
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusSucceeded {
			add(a.Image)
		}
	}
	if rt.deployGate != nil {
		pins, err := rt.deployGate.ListRollbackPins(ctx, svc.Name)
		if err != nil {
			return nil, fmt.Errorf("list rollback pins: %w", err)
		}
		for _, p := range pins {
			add(p.Image)
		}
	}
	return out, nil
}

// checkRollbackTarget answers 400 with the valid targets when image was never
// a previous deploy of svc. It reports whether the request may continue.
func (rt *Router) checkRollbackTarget(w http.ResponseWriter, r *http.Request, svc store.DesiredService, image string) bool {
	want := unpinnedRef(image)
	if want == unpinnedRef(svc.Image) {
		return true
	}
	targets, err := rt.rollbackTargets(r.Context(), svc)
	if err != nil {
		rt.internalError(w, "api: rollback preflight failed", err)
		return false
	}
	for _, t := range targets {
		if unpinnedRef(t) == want {
			return true
		}
	}
	if targets == nil {
		targets = []string{}
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":         fmt.Sprintf("image %q was never a deploy of app %q; roll back to one of the valid targets", image, svc.Name),
		"code":          errCodeRollbackTargetUnknown,
		"valid_targets": targets,
	})
	return false
}
