package api

import (
	"context"
	"time"

	"github.com/glincker/levelrail/internal/backup"
	"github.com/glincker/levelrail/internal/iac"
	"github.com/glincker/levelrail/internal/models"
	"github.com/glincker/levelrail/internal/placement"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// attachLiveLimits points the request limiters at the runtime settings so a saved change reaches the next request.
func (rt *Router) attachLiveLimits() {
	if rt.runtimeSettings == nil {
		return
	}
	rt.attachGuardSources()
	if rt.apiRateLimit != nil {
		rt.limitSource(rt.apiRateLimit.reads, runtimesettings.KeyAPIRateLimitRead)
		rt.limitSource(rt.apiRateLimit.writes, runtimesettings.KeyAPIRateLimitWrite)
	}
	if rt.tokenRedeemRateLimit != nil {
		rt.limitSource(rt.tokenRedeemRateLimit, runtimesettings.KeyAPIRateLimitRedeem)
	}
}

// limitSource reads the clamped setting every time, so an env seed of 0 or a value under the floor never weakens a limiter and a reset restores the seed.
func (rt *Router) limitSource(l *apiRateLimiter, key string) {
	l.source = func() int { return rt.runtimeSettings.Limit(context.Background(), key) }
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
	// With nothing saved or in env the option value (an embedder's cap) stands; boot passes the clamped env value there.
	if rt.runtimeSettings == nil || rt.runtimeSettings.Source(ctx, runtimesettings.KeyDeployMaxConcurrent) == runtimesettings.SourceDefault {
		return rt.deployMaxConcurrent
	}
	return rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDeployMaxConcurrent)
}

// attachGuardSources points the lower packages' limits at the runtime settings; each reads the clamped value per use.
func (rt *Router) attachGuardSources() {
	lim := func(key string) func() int {
		return func() int { return rt.runtimeSettings.Limit(context.Background(), key) }
	}
	store.SetSecretMatchMinSource(lim(runtimesettings.KeyIaCSecretMatchMinLen))
	iac.SetPlanValueSource(lim(runtimesettings.KeyIaCPlanValueMax))
	pct, cnt := lim(runtimesettings.KeyStackPruneMaxPercent), lim(runtimesettings.KeyStackPruneMaxCount)
	iac.SetPruneGuardSource(func() iac.PruneGuard { return iac.PruneGuard{MaxPercent: pct(), MaxCount: cnt()} })
	iac.SetV1HashAcceptedSource(func() bool {
		return rt.runtimeSettings.Bool(context.Background(), runtimesettings.KeyIaCV1HashAccepted)
	})
	iac.SetComposeSource(func() iac.ComposeLimits {
		ctx := context.Background()
		return iac.ComposeLimits{
			Enabled:       rt.runtimeSettings.Bool(ctx, runtimesettings.KeyIaCComposeRefs),
			MaxFileBytes:  rt.runtimeSettings.Limit(ctx, runtimesettings.KeyIaCComposeMaxFileKB) << 10,
			MaxFiles:      rt.runtimeSettings.Limit(ctx, runtimesettings.KeyIaCComposeMaxFiles),
			MaxTotalBytes: rt.runtimeSettings.Limit(ctx, runtimesettings.KeyIaCComposeMaxTotalKB) << 10,
		}
	})
	placement.SetAffinityTermsSource(lim(runtimesettings.KeyPlacementAffinityTerms))
	backup.SetWebDAVSource(func() backup.WebDAVSettings {
		ctx, rs := context.Background(), rt.runtimeSettings
		return backup.WebDAVSettings{
			AllowPrivate:  rs.Bool(ctx, runtimesettings.KeyBackupWebDAVAllowPrivate),
			HeaderTimeout: rs.Duration(ctx, runtimesettings.KeyBackupWebDAVHeaderTimeout),
			Idle:          rs.Duration(ctx, runtimesettings.KeyBackupWebDAVIdleTimeout),
			Request:       rs.Duration(ctx, runtimesettings.KeyBackupWebDAVRequestTimeout),
			MaxResponse:   int64(rs.Limit(ctx, runtimesettings.KeyBackupWebDAVMaxResponse)),
			MaxEntries:    rs.Limit(ctx, runtimesettings.KeyBackupWebDAVMaxEntries),
			MaxDepth:      rs.Limit(ctx, runtimesettings.KeyBackupWebDAVMaxDepth),
		}
	})
	backup.SetSFTPSource(func() backup.SFTPSettings {
		ctx, rs := context.Background(), rt.runtimeSettings
		return backup.SFTPSettings{
			AllowPrivate: rs.Bool(ctx, runtimesettings.KeyBackupSFTPAllowPrivate),
			Connect:      rs.Duration(ctx, runtimesettings.KeyBackupSFTPConnectTimeout),
			Idle:         rs.Duration(ctx, runtimesettings.KeyBackupSFTPIdleTimeout),
			MaxEntries:   rs.Limit(ctx, runtimesettings.KeyBackupSFTPMaxEntries),
			MaxDepth:     rs.Limit(ctx, runtimesettings.KeyBackupSFTPMaxDepth),
			MaxConns:     rs.Limit(ctx, runtimesettings.KeyBackupSFTPMaxConns),
			MaxPerClient: 8,
		}
	})
	models.SetHFLimitSource(func() (time.Duration, int64) {
		sec := rt.runtimeSettings.Limit(context.Background(), runtimesettings.KeyHFTimeoutSeconds)
		return time.Duration(sec) * time.Second, int64(rt.runtimeSettings.Limit(context.Background(), runtimesettings.KeyHFMaxResponseBytes))
	})
}
