package runtimesettings

// Registered keys for the cloud provisioning engine guards. Env names are seeds only.
const (
	KeyCloudMaxDestroys           = "cloud.max_destroys"
	KeyCloudAllowProtectedDelete  = "cloud.allow_protected_delete"
	KeyCloudRequireFullEstimate   = "cloud.require_full_estimate"
	KeyCloudEngineTimeoutSeconds  = "cloud.engine_timeout_seconds"
	KeyCloudDefaultMonthlyCostLim = "cloud.default_monthly_cost_limit"
)

func init() {
	Register(Definition{
		Key: KeyCloudMaxDestroys, Category: CategoryDeploys, Kind: KindInt, Default: "0", Env: "APP_CLOUD_MAX_DESTROYS", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, 10000),
		Title:       "Cloud destroys allowed per plan",
		Description: "How many resources one cloud plan may delete or replace before it is refused, from 0 to 10000. 0 refuses any destroy. The next plan uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudAllowProtectedDelete, Category: CategorySecurity, Kind: KindBool, Default: "false", Env: "APP_CLOUD_ALLOW_PROTECTED_DELETE", Apply: ApplyLive,
		Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow deleting cloud data resources",
		Description: "Let a cloud plan delete buckets, volumes and databases. Off refuses those deletes. The next plan uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudRequireFullEstimate, Category: CategorySecurity, Kind: KindBool, Default: "true", Env: "APP_CLOUD_REQUIRE_FULL_ESTIMATE", Apply: ApplyLive,
		Risk:        RiskWeakensSecurity,
		Title:       "Require a full cost estimate",
		Description: "Refuse a cloud plan that creates resources without a known price. Turning it off lets unpriced resources through the cost limit. The next plan uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudEngineTimeoutSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "1800", Env: "APP_CLOUD_ENGINE_TIMEOUT_SECONDS", Apply: ApplyLive,
		CheckValue:  intRangeCheck(60, 86400),
		Title:       "Cloud engine run timeout",
		Description: "Seconds a cloud plan or apply may run before it is stopped, from 60 to 86400. The next run uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudDefaultMonthlyCostLim, Category: CategoryDeploys, Kind: KindInt, Default: "0", Env: "APP_CLOUD_DEFAULT_MONTHLY_COST_LIMIT", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, 1000000000),
		Title:       "Default monthly cloud cost limit",
		Description: "Monthly spend limit in whole currency units for a cloud account that has none of its own. 0 means a limit must be set per account. The next plan uses a changed value.",
	})
}
