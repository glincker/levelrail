package ingress

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithNextGraces enables Next.js skew routing: graces returns a service's
// superseded releases (deployment id plus the image it ran) still inside their grace window.
func WithNextGraces(graces func(service string, now time.Time) []store.NextAssetGrace) Option {
	return func(c *Controller) { c.nextGraces = graces }
}

// nextSkewFor maps each graced deployment id to the container of its own
// release. The held containers are not in the normal pool: only requests that
// carry an old id reach them, and no replica of the current release is ever a target.
func (c *Controller) nextSkewFor(ctx context.Context, svc store.DesiredService, currentDial string, now time.Time) []ingress.NextSkewRoute {
	if c.nextGraces == nil {
		return nil
	}
	graces := c.nextGraces(svc.Name, now)
	if len(graces) == 0 {
		return nil
	}
	containers, err := c.listLocal(ctx, svc.Name+"-")
	if err != nil {
		return nil
	}
	current := application.ReleaseKey(svc.Name, application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce))
	var held []docker.ContainerState
	for _, cs := range c.rankReleases(ctx, svc.Name, "", containers) {
		if application.ReleaseKey(svc.Name, cs.Name) != current {
			held = append(held, cs)
		}
	}
	var out []ingress.NextSkewRoute
	byDial := map[string]int{}
	for _, g := range graces {
		dial, ok := heldDialFor(held, g.Image, svc.Port)
		if !ok || dial == currentDial {
			continue
		}
		i, seen := byDial[dial]
		if !seen {
			i = len(out)
			byDial[dial] = i
			out = append(out, ingress.NextSkewRoute{HeldDial: dial})
		}
		out[i].DeploymentIDs = append(out[i].DeploymentIDs, g.DeploymentID)
	}
	return out
}

// heldDialFor picks the held container running image, else the best ranked held
// one (a rebuilt tag keeps the same image name across releases).
func heldDialFor(held []docker.ContainerState, image string, port int) (string, bool) {
	want := docker.CanonicalImageRef(image)
	for i := range held {
		if image != "" && docker.CanonicalImageRef(held[i].Image) == want {
			if dial, ok := dialAddr(&held[i], port); ok {
				return dial, true
			}
		}
	}
	for i := range held {
		if dial, ok := dialAddr(&held[i], port); ok {
			return dial, true
		}
	}
	return "", false
}
