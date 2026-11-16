package runtimesettings

// Docker Hub integration keys.
const (
	KeyDockerHubLimitWarnPercent  = "registry.dockerhub.limit_warn_percent"
	KeyDockerHubLimitsCacheSecond = "registry.dockerhub.limits_cache_seconds"
	KeyDockerHubTagPageSize       = "registry.dockerhub.tag_page_size"
	KeyDockerHubUseCredential     = "registry.dockerhub.use_credential"
)

func init() {
	registerLimit(Definition{
		Key: KeyDockerHubLimitWarnPercent, Category: CategoryDeploys, Default: "20", Env: "APP_REGISTRY_DOCKERHUB_LIMIT_WARN_PERCENT", Apply: ApplyLive,
		Title:       "Docker Hub pull limit warning",
		Description: "Warn when the remaining Docker Hub pulls fall below this percent of the limit, from 0 to 100. The next check uses a changed value.",
	}, 0, 100)
	registerLimit(Definition{
		Key: KeyDockerHubLimitsCacheSecond, Category: CategoryDeploys, Default: "300", Env: "APP_REGISTRY_DOCKERHUB_LIMITS_CACHE_SECONDS", Apply: ApplyLive,
		Title:       "Docker Hub limit check cache",
		Description: "How long a Docker Hub pull limit reading is reused, from 10 to 3600 seconds. The reading costs no pull quota.",
	}, 10, 3600)
	registerLimit(Definition{
		Key: KeyDockerHubTagPageSize, Category: CategoryDeploys, Default: "25", Env: "APP_REGISTRY_DOCKERHUB_TAG_PAGE_SIZE", Apply: ApplyLive,
		Title:       "Docker Hub browser page size",
		Description: "Most repositories or tags one Docker Hub browser request lists, from 1 to 100. The next request uses a changed value.",
	}, 1, 100)
	Register(Definition{
		Key: KeyDockerHubUseCredential, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Env: "APP_REGISTRY_DOCKERHUB_USE_CREDENTIAL", Apply: ApplyLive,
		Title:       "Use the Docker Hub token for docker.io pulls",
		Description: "When on, a connected Docker Hub token authenticates pulls and digest lookups from docker.io, which raises the pull limit. Turn it off to pull anonymously again. The environment variable only seeds the default.",
	})
}
