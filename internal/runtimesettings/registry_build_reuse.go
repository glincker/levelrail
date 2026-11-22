package runtimesettings

// KeyBuildReuseUnchanged turns image reuse for an unchanged commit and build settings on or off.
const KeyBuildReuseUnchanged = "build.reuse_unchanged"

func init() {
	Register(Definition{
		Key: KeyBuildReuseUnchanged, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Apply:    ApplyLive,
		TitleKey: "runtimeSettings:items.buildReuseUnchanged.title", DescKey: "runtimeSettings:items.buildReuseUnchanged.description",
		Title:       "Reuse the image for an unchanged commit",
		Description: "Redeploying the same commit with the same build settings reuses the image already built on this node instead of rebuilding. A no-cache build, a build with secrets and an attested build always rebuild. The next deploy uses a changed value.",
	})
}

// KeyBuildRunnerMinDiskMB is the free disk a build node needs to be picked for a build.
const KeyBuildRunnerMinDiskMB = "build.runner_min_disk_free_mb"

func init() {
	registerLimit(Definition{
		Key: KeyBuildRunnerMinDiskMB, Category: CategoryDeploys, Default: "2048", Apply: ApplyLive,
		TitleKey: "runtimeSettings:items.buildRunnerMinDisk.title", DescKey: "runtimeSettings:items.buildRunnerMinDisk.description",
		Title:       "Build runner minimum free disk",
		Description: "A build node reporting less free disk than this many MiB is skipped for new builds and shown as skipped on the Runners panel. 0 turns the gate off. The next build uses a changed value.",
	}, 0, 10485760)
}
