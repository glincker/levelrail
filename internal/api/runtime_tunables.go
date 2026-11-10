package api

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cdn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// tunableInt reads a runtime setting; a router without settings keeps the old env-then-default path.
func (rt *Router) tunableInt(ctx context.Context, key, env string, def int) int {
	if rt.runtimeSettings != nil {
		if n := rt.runtimeSettings.Int(ctx, key); n > 0 {
			return n
		}
		return def
	}
	return envIntDefault(env, def)
}

func (rt *Router) tunableDuration(ctx context.Context, key, env string, def time.Duration) time.Duration {
	if rt.runtimeSettings != nil {
		if d := rt.runtimeSettings.Duration(ctx, key); d > 0 {
			return d
		}
		return def
	}
	return envDuration(env, def)
}

func (rt *Router) cloneMaxApps(ctx context.Context) int {
	return rt.tunableInt(ctx, runtimesettings.KeyProjectCloneMaxApps, envProjectCloneMaxApps, defaultProjectCloneMaxApps)
}

func (rt *Router) clonePlaceholderSuffix(ctx context.Context) string {
	suffix := ""
	if rt.runtimeSettings != nil {
		suffix = rt.runtimeSettings.String(ctx, runtimesettings.KeyProjectClonePlaceholderDomain)
	} else {
		suffix = os.Getenv(envProjectClonePlaceholder)
	}
	if suffix = strings.TrimSpace(suffix); suffix != "" {
		return suffix
	}
	return defaultCloneDomainSuffix
}

func (rt *Router) previewMigratePlatformTimeout(ctx context.Context) time.Duration {
	return rt.tunableDuration(ctx, runtimesettings.KeyPreviewMigrateTimeout, envPreviewMigrateTimeout, defaultPreviewMigrateTimeout)
}

func (rt *Router) previewMigrateWait(ctx context.Context) time.Duration {
	return rt.tunableDuration(ctx, runtimesettings.KeyPreviewMigrateWait, envPreviewMigrateWait, defaultPreviewMigrateWait)
}

func (rt *Router) deployExplainLines(ctx context.Context) int {
	return rt.tunableInt(ctx, runtimesettings.KeyDeployExplainLogLines, envDeployExplainLines, defaultDeployExplainLines)
}

// procfileWebPort returns the live Procfile web port, or 0 to keep the translator default.
func (rt *Router) procfileWebPort(ctx context.Context) int {
	if rt.runtimeSettings == nil {
		return 0
	}
	return rt.runtimeSettings.Int(ctx, runtimesettings.KeyDeployProcfileDefaultPort)
}

// cdnPurgeHistory returns the shared purge history with its cap refreshed from the setting.
func (rt *Router) cdnPurgeHistory(ctx context.Context) *cdn.History {
	h := cdn.DefaultHistory()
	if rt.runtimeSettings != nil {
		h.SetKeep(rt.runtimeSettings.Int(ctx, runtimesettings.KeyCDNPurgeHistory))
	}
	return h
}

func (rt *Router) offeredUpdateChannels(ctx context.Context) []string {
	if rt.runtimeSettings == nil {
		return offeredUpdateChannels()
	}
	out := []string{}
	for _, c := range rt.runtimeSettings.List(ctx, runtimesettings.KeyUpdateOfferedChannels) {
		if upgrade.ValidChannel(c) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return []string{upgrade.ChannelStable}
	}
	return out
}
