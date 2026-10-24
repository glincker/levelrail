package ingress

import (
	"context"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// failoverEnv is how long a request may keep retrying the held previous
// release after the new one fails to accept it. "off" or empty disables.
const failoverEnv = "LEVELRAIL_INGRESS_FAILOVER_TRY_DURATION"

// failoverDefault applies when the env var is unset.
const failoverDefault = "5s"

func failoverWindow() string {
	v, set := os.LookupEnv(failoverEnv)
	v = strings.TrimSpace(v)
	if !set {
		return failoverDefault
	}
	if strings.EqualFold(v, "off") {
		return ""
	}
	return v
}

// failoverRoute keeps the held previous release as a second upstream so a
// release that crashes after readiness fails over on the first refused
// dial instead of waiting for the next reconcile. nil means single upstream.
func (c *Controller) failoverRoute(ctx context.Context, svc store.DesiredService, dial string) *ingress.LBRoute {
	window := failoverWindow()
	if window == "" {
		return nil
	}
	target := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	prev, err := c.previousReleases(ctx, svc.Name, target)
	if err != nil || len(prev) == 0 {
		return nil
	}
	cur, err := c.inspectLocal(ctx, target)
	if err != nil || cur == nil {
		return nil
	}
	prevDial, ok := "", false
	for i := range prev {
		// Same image means a sibling replica, not a previous release.
		if prev[i].Image == cur.Image {
			continue
		}
		if d, found := dialAddr(&prev[i]); found && d != dial {
			prevDial, ok = d, true
			break
		}
	}
	if !ok {
		return nil
	}
	return &ingress.LBRoute{
		Upstreams:     []string{dial, prevDial},
		Policy:        ingress.LBPolicyFirst,
		Retries:       0,
		TryDuration:   window,
		TryInterval:   "250ms",
		PassiveHealth: &ingress.PassiveHealthChecks{FailDuration: window, MaxFails: 1},
	}
}
