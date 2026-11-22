package runtimesettings

// KeyDeployMaxConcurrentPerProject caps simultaneous builds inside one project.
const KeyDeployMaxConcurrentPerProject = "deploy.max_concurrent_per_project"

// KeyBuildFairShare makes queued builds take turns across projects.
const KeyBuildFairShare = "build.fair_share"

func init() {
	Register(Definition{
		Key: KeyBuildFairShare, Category: CategoryDeploys, Kind: KindBool, Default: "true", Env: "APP_BUILD_FAIR_SHARE", Apply: ApplyLive,
		Title:       "Fair build queue across projects",
		Description: "When builds queue for a slot, the next slot goes to the project with the fewest builds running, so one project's burst does not block the others. Off means plain first come first served. A change applies to the next queue decision.",
	})
	registerLimit(Definition{
		Key: KeyDeployMaxConcurrentPerProject, Category: CategoryDeploys, Default: "0", Env: "APP_DEPLOY_MAX_CONCURRENT_PER_PROJECT", Apply: ApplyLive,
		Title:       "Concurrent deploys per project",
		Description: "How many deploys of apps in one project may run at once; extra deploys wait in the queue and start when one finishes. 0 means no limit. The host-wide limit still applies. A change applies to the next queue decision.",
	}, 0, deployMaxConcurrentCeiling)
}
