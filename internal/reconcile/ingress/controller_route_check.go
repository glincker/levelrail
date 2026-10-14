package ingress

import "github.com/GLINCKER/levelrail/internal/ingress"

// WithRouteCheck makes every host answer the proxy route verifier's marker,
// so a fronting proxy's route can be proven to reach this ingress.
func WithRouteCheck(rc ingress.RouteCheck) Option {
	return func(c *Controller) {
		if rc.Valid() {
			c.routeCheck = &rc
		}
	}
}
