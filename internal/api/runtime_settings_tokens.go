package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func (rt *Router) tokenSettingInt(ctx context.Context, key string) int {
	svc := rt.runtimeSettings
	if svc == nil {
		svc = runtimesettings.New(nil, func(string) (string, bool) { return "", false })
	}
	return svc.Int(ctx, key)
}

// tokenDefaultTTLDays is the operator's default lifetime, 0 when unset.
func (rt *Router) tokenDefaultTTLDays(ctx context.Context) int {
	return rt.tokenSettingInt(ctx, runtimesettings.KeyTokensDefaultTTLDays)
}

// tokenUnusedAlertDays is the idle threshold for the unused-token alert, 0 when off.
func (rt *Router) tokenUnusedAlertDays(ctx context.Context) int {
	return rt.tokenSettingInt(ctx, runtimesettings.KeyTokensUnusedAlertDays)
}
