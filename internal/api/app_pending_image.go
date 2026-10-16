package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// lastServingImage returns the image of the newest attempt the controller
// confirmed serving, when the app's newest attempt has not reached that
// state yet (rolling out or failed readiness). ok is false when the desired
// image is already the serving one or there is nothing to fall back to.
func (rt *Router) lastServingImage(ctx context.Context, svc store.DesiredService) (image string, ok bool) {
	if rt.deployAttempts == nil {
		return "", false
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, svc.Name)
	if err != nil || len(attempts) == 0 {
		return "", false
	}
	return pickServingImage(attempts, svc.Image)
}

// pickServingImage expects attempts newest first.
func pickServingImage(attempts []store.DeployAttempt, desired string) (string, bool) {
	newest := attempts[0]
	if newest.Image != desired || newest.RolloutState == store.RolloutStateServing || isStaticAttempt(newest) {
		return "", false
	}
	if newest.Status != store.DeployAttemptStatusSucceeded && newest.Status != store.DeployAttemptStatusRunning {
		return "", false
	}
	for _, a := range attempts[1:] {
		if a.RolloutState == store.RolloutStateServing && a.Image != "" && a.Image != desired {
			return a.Image, true
		}
	}
	return "", false
}
