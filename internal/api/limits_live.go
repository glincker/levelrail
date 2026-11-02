package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// attachLiveLimits points the request limiters at the runtime settings so a saved change reaches the next request.
func (rt *Router) attachLiveLimits() {
	if rt.runtimeSettings == nil {
		return
	}
	if rt.apiRateLimit != nil {
		rt.limitSource(rt.apiRateLimit.reads, runtimesettings.KeyAPIRateLimitRead)
		rt.limitSource(rt.apiRateLimit.writes, runtimesettings.KeyAPIRateLimitWrite)
	}
	if rt.tokenRedeemRateLimit != nil {
		rt.limitSource(rt.tokenRedeemRateLimit, runtimesettings.KeyAPIRateLimitRedeem)
	}
}

// limitSource lets a saved value win and otherwise keeps the construction value (the env seed or default), so a reset restores it.
func (rt *Router) limitSource(l *apiRateLimiter, key string) {
	seed := int(l.ratePerMinute)
	l.source = func() int {
		if rt.runtimeSettings.Source(context.Background(), key) == runtimesettings.SourceSaved {
			return rt.runtimeSettings.Limit(context.Background(), key)
		}
		return seed
	}
}

// maxBodyBytes is the live API body backstop; a router without settings serves the registry default and env seed.
func (rt *Router) maxBodyBytes(ctx context.Context) int64 {
	return int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyAPIMaxBodyBytes))
}

// assetsUploadMax is the live per-object upload cap.
func (rt *Router) assetsUploadMax(ctx context.Context) int64 {
	return int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyAssetsUploadMaxBytes))
}

// deployCap is the live deploy concurrency limit; 0 means unlimited.
func (rt *Router) deployCap(ctx context.Context) int {
	if rt.runtimeSettings == nil || rt.runtimeSettings.Source(ctx, runtimesettings.KeyDeployMaxConcurrent) != runtimesettings.SourceSaved {
		return rt.deployMaxConcurrent
	}
	return rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDeployMaxConcurrent)
}
