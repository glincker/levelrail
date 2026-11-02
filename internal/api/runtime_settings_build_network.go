package api

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Registered here so internal/build does not depend on the settings package.
func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: build.KeyBuildNetwork, Category: runtimesettings.CategorySecurity, Kind: runtimesettings.KindString,
		Default: string(build.NetworkOpen), Env: build.EnvBuildNetwork, Apply: runtimesettings.ApplyLive,
		AllowedStrings: []string{string(build.NetworkOpen), string(build.NetworkNone), string(build.NetworkBlockPrivate)},
		StrengthOrder:  []string{string(build.NetworkOpen), string(build.NetworkBlockPrivate), string(build.NetworkNone)},
		Risk:           runtimesettings.RiskWeakensSecurity,
		FailClosed:     true,
		EnvConvert:     func(raw string) (string, error) { return strings.ToLower(strings.TrimSpace(raw)), nil },
		TitleKey:       "runtimeSettings:items.buildNetwork.title",
		DescKey:        "runtimeSettings:items.buildNetwork.description",
		Title:          "Build network",
		Description:    "Network for RUN steps of a build: open (no restriction), none, or block-private (reserved: builds fail until it can be enforced). Read on every build; the environment variable only seeds the default.",
	})
}
