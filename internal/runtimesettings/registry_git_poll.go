package runtimesettings

// Keys of the live git poller settings.
const (
	KeyGitPollEnabled     = "git.poll_enabled"
	KeyGitPollInterval    = "git.poll_interval"
	KeyGitPollConcurrency = "git.poll_concurrency"
	KeyGitPollMaxBackoff  = "git.poll_max_backoff"
)

func init() {
	Register(Definition{
		Key: KeyGitPollEnabled, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Env: "APP_GIT_POLL_ENABLED", Apply: ApplyLive,
		Title:       "Poll git sources for missed pushes",
		Description: "Compare each auto-deploy git source with its remote branch and queue a deploy when a webhook was missed.",
	})
	Register(Definition{
		Key: KeyGitPollInterval, Category: CategoryDeploys, Kind: KindDuration, Default: "5m",
		Env: "APP_GIT_POLL_INTERVAL", Apply: ApplyLive,
		Title:       "Git poll interval",
		Description: "How often each git source is checked for a missed push.",
	})
	Register(Definition{
		Key: KeyGitPollConcurrency, Category: CategoryDeploys, Kind: KindInt, Default: "4",
		Env: "APP_GIT_POLL_CONCURRENCY", Apply: ApplyLive,
		Title:       "Git poll concurrency",
		Description: "How many git sources are checked at the same time.",
	})
	Register(Definition{
		Key: KeyGitPollMaxBackoff, Category: CategoryDeploys, Kind: KindDuration, Default: "1h",
		Env: "APP_GIT_POLL_MAX_BACKOFF", Apply: ApplyLive,
		Title:       "Git poll max backoff",
		Description: "The longest wait before a failing or rate limited git source is checked again.",
	})
}
