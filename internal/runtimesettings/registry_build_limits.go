package runtimesettings

// KeyDeployMaxConcurrentPerProject caps simultaneous builds inside one project.
const KeyDeployMaxConcurrentPerProject = "deploy.max_concurrent_per_project"

func init() {
	registerLimit(Definition{
		Key: KeyDeployMaxConcurrentPerProject, Category: CategoryDeploys, Default: "0", Env: "APP_DEPLOY_MAX_CONCURRENT_PER_PROJECT", Apply: ApplyLive,
		Title:       "Concurrent deploys per project",
		Description: "How many deploys of apps in one project may run at once; extra deploys wait in the queue and start when one finishes. 0 means no limit. The host-wide limit still applies. A change applies to the next queue decision.",
	}, 0, deployMaxConcurrentCeiling)
}
