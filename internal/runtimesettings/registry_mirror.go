package runtimesettings

import "github.com/GLINCKER/levelrail/internal/runtimesettings/registrymirror"

// KeyRegistryMirrorPrefix is the pull-through prefix for the agent and BuildKit images.
const KeyRegistryMirrorPrefix = "registry.mirror_prefix"

func init() {
	Register(Definition{
		Key: KeyRegistryMirrorPrefix, Category: CategoryDeploys, Kind: KindString, Default: "",
		Env: "APP_REGISTRY_MIRROR_PREFIX", Apply: ApplyLive,
		CheckValue:  registrymirror.Validate,
		Title:       "Registry mirror prefix",
		Description: "A hostname, with an optional path and no scheme, put in front of the registry host when pulling the node agent and BuildKit images, for example m.daocloud.io. Digest pins are kept. Empty pulls directly. Read when a node is provisioned or the build daemon starts; the environment variable only seeds the default.",
	})
}
