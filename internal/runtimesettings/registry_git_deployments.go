package runtimesettings

// Keys for reporting deploys to a GitLab project's Deployments and Environments pages.
const (
	KeyGitLabDeployments            = "git.gitlab_deployments"
	KeyGitLabDeploymentsConnections = "git.gitlab_deployments_connections"
)

func init() {
	Register(Definition{
		Key: KeyGitLabDeployments, Category: CategoryDeploys, Kind: KindBool, Default: "false",
		Env: "APP_GIT_GITLAB_DEPLOYMENTS", Apply: ApplyLive,
		Title:       "Report deploys to GitLab environments",
		Description: "Create a GitLab environment per app environment and a deployment per deploy. Needs a connection token with the api scope. Off by default. The next deploy uses a changed value.",
	})
	Register(Definition{
		Key: KeyGitLabDeploymentsConnections, Category: CategoryDeploys, Kind: KindStringList, Default: "",
		Env: "APP_GIT_GITLAB_DEPLOYMENTS_CONNECTIONS", Apply: ApplyLive,
		Title:       "GitLab connections that get deployment reports",
		Description: "Git connection ids that report deploys when the setting above is on. Empty means every GitLab connection.",
	})
}
