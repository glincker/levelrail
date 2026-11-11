package application

import (
	"context"
	"log/slog"
	"slices"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithElevatedCaps sets where the live "elevated capabilities allowed" setting comes
// from. It is read on every pass, so turning the tier off revokes at the next reconcile.
func WithElevatedCaps(get func(context.Context) bool) Option {
	return func(ctrl *Controller) { ctrl.elevatedCaps = get }
}

// withPolicyCaps returns desired with stored grants cut down to what policy allows now.
// Elevated caps need the setting on and no egress policy (the sidecar shares the netns, so
// NET_ADMIN could flush its rules); with no reader wired it fails closed.
func (c *Controller) withPolicyCaps(ctx context.Context, desired *store.DesiredService) *store.DesiredService {
	if len(desired.GrantedCaps) == 0 {
		return desired
	}
	allowElevated := c.elevatedCaps != nil && c.elevatedCaps(ctx) && desired.Egress == nil
	kept := make([]string, 0, len(desired.GrantedCaps))
	for _, g := range desired.GrantedCaps {
		if slices.Contains(dockerguard.GrantableCaps, g) && !allowElevated {
			continue
		}
		kept = append(kept, g)
	}
	if len(kept) == len(desired.GrantedCaps) {
		return desired
	}
	slog.Warn("reconcile: elevated capability grant not applied", slog.String("service", c.serviceName), slog.Bool("egress_policy", desired.Egress != nil))
	cp := *desired
	cp.GrantedCaps = kept
	if len(kept) == 0 {
		cp.GrantedCaps = nil
	}
	return &cp
}
