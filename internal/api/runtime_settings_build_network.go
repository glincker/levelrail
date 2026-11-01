package api

import (
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Registered here so internal/build does not depend on the settings package.
func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: build.KeyBuildNetwork, Category: runtimesettings.CategorySecurity, Kind: runtimesettings.KindString,
		Default: string(build.NetworkOpen), Env: build.EnvBuildNetwork, Apply: runtimesettings.ApplyLive,
		AllowedStrings: []string{string(build.NetworkOpen), string(build.NetworkNone), string(build.NetworkBlockPrivate)},
		Title:          "Build network",
		Description:    "Network for RUN steps of a build: open (no restriction), none, or block-private (reserved: builds fail until it can be enforced). Read on every build; the environment variable only seeds the default.",
	})
}
