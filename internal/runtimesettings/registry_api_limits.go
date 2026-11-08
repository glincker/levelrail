package runtimesettings

// Registered keys for API limits that used to be env-only.
const (
	KeyAppsMetricsMax                = "api.apps_metrics_max"
	KeyLogExportMaxLines             = "logs.export_max_lines"
	KeyStackHistoryLimit             = "stacks.history_limit"
	KeyMaxTagsPerApp                 = "tags.max_per_app"
	KeyFixitMaxPerHour               = "fixit.max_per_hour"
	KeyDeployApprovalClaimTimeoutSec = "deploy.approval_claim_timeout_seconds"
	KeyApproveDigestCheckTimeoutSec  = "deploy.approve_digest_check_timeout_seconds"
)

func init() {
	registerLimit(Definition{
		Key: KeyAppsMetricsMax, Category: CategoryObservability, Default: "200", Env: "APP_APPS_METRICS_MAX", Apply: ApplyLive,
		Title:       "Apps metrics batch size",
		Description: "Most apps one batch metrics request returns; the rest are left out. From 1 to 5000. The next request uses a changed value.",
	}, 1, 5000)
	registerLimit(Definition{
		Key: KeyLogExportMaxLines, Category: CategoryObservability, Default: "5000", Env: "APP_LOG_EXPORT_MAX_LINES", Apply: ApplyLive,
		Title:       "Log download line cap",
		Description: "Most log lines one log download returns, newest kept. From 100 to 1000000. The next download uses a changed value.",
	}, 100, 1_000_000)
	registerLimit(Definition{
		Key: KeyStackHistoryLimit, Category: CategoryDeploys, Default: "50", Env: "APP_STACK_HISTORY_LIMIT", Apply: ApplyLive,
		Title:       "Stack revision history",
		Description: "Most stack revisions listed per stack. From 1 to 1000. The next request uses a changed value.",
	}, 1, 1000)
	registerLimit(Definition{
		Key: KeyMaxTagsPerApp, Category: CategoryDeploys, Default: "20", Env: "APP_MAX_TAGS_PER_APP", Apply: ApplyLive,
		Title:       "Tags per app",
		Description: "Most tags one app may carry. From 1 to 500. Tags already attached are kept when the value is lowered.",
	}, 1, 500)
	registerLimit(Definition{
		Key: KeyFixitMaxPerHour, Category: CategorySecurity, Default: "0", Env: "APP_FIXIT_MAX_PER_HOUR", Apply: ApplyRestart,
		Title:       "Fix-it applies per hour",
		Description: "Most times one fix may be applied per hour. 0 keeps the built-in cap. Read when the fix engine starts, so a change needs a restart.",
	}, 0, 1000)
	registerLimit(Definition{
		Key: KeyDeployApprovalClaimTimeoutSec, Category: CategoryDeploys, Default: "900", Env: "APP_DEPLOY_APPROVAL_CLAIM_TIMEOUT_SECONDS", Apply: ApplyLive,
		Title:       "Deploy approval claim timeout",
		Description: "Seconds an approving claim may stand before recovery treats its claimer as gone. From 30 to 86400. The next recovery pass uses a changed value.",
	}, 30, 86400)
	registerLimit(Definition{
		Key: KeyApproveDigestCheckTimeoutSec, Category: CategoryDeploys, Default: "0", Env: "APP_APPROVE_DIGEST_CHECK_TIMEOUT_SECONDS", Apply: ApplyLive,
		Title:       "Approval digest check timeout",
		Description: "Seconds the registry digest check may take when an approval is granted. 0 uses the request deadline. From 0 to 600.",
	}, 0, 600)
}
