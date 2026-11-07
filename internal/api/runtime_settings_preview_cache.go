package api

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: build.KeyPreviewCacheMode, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindString,
		Default: build.PreviewCacheIsolated, Env: build.EnvPreviewCacheMode, Apply: runtimesettings.ApplyLive,
		AllowedStrings: []string{build.PreviewCacheIsolated, build.PreviewCacheSharedWithApp, build.PreviewCacheOff},
		Risk:           runtimesettings.RiskNormal,
		EnvConvert:     func(raw string) (string, error) { return strings.ToLower(strings.TrimSpace(raw)), nil },
		Title:          "Preview build cache",
		Description:    "How pull request preview builds use the BuildKit cache. isolated gives each preview its own cache, shared_with_app reuses the app's cache (faster, but a preview can poison the production cache), off builds every preview with a fresh cache. Read on every preview build; the environment variable only seeds the default.",
	})
}
