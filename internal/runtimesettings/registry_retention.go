package runtimesettings

// Built-in registry retention keys.
const (
	KeyRegistryKeepTags = "retention.registry_keep_tags"
	KeyRegistryGCMinMB  = "retention.registry_gc_min_mb"
)

func init() {
	Register(Definition{
		Key: KeyRegistryKeepTags, Category: CategoryRetention, Kind: KindInt, Default: "0",
		Apply: ApplyLive, CheckValue: NonNegativeCheck,
		TitleKey: "runtimeSettings:items." + KeyRegistryKeepTags + ".title", DescKey: "runtimeSettings:items." + KeyRegistryKeepTags + ".description",
		Title:       "Keep this many newest tags per registry repository",
		Description: "Older tags in the built-in registry are removed by cleanup. A tag an app uses is never removed. 0 keeps every tag.",
	})
	Register(Definition{
		Key: KeyRegistryGCMinMB, Category: CategoryRetention, Kind: KindInt, Default: "100",
		Apply: ApplyLive, CheckValue: NonNegativeCheck,
		TitleKey: "runtimeSettings:items." + KeyRegistryGCMinMB + ".title", DescKey: "runtimeSettings:items." + KeyRegistryGCMinMB + ".description",
		Title:       "Reclaim registry space once unreferenced data reaches (MB)",
		Description: "Cleanup runs registry garbage collection only when at least this many megabytes of layers are unreferenced. 0 disables it.",
	})
}
