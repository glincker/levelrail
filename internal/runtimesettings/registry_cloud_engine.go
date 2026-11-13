package runtimesettings

// Registered keys for the cloud provisioning engine guards. Env names are seeds only.
const (
	KeyCloudMaxDestroys           = "cloud.max_destroys"
	KeyCloudAllowProtectedDelete  = "cloud.allow_protected_delete"
	KeyCloudRequireFullEstimate   = "cloud.require_full_estimate"
	KeyCloudEngineTimeoutSeconds  = "cloud.engine_timeout_seconds"
	KeyCloudDefaultMonthlyCostLim = "cloud.default_monthly_cost_limit"
	KeyCloudFetchMaxMB            = "cloud.fetch_max_mb"
	KeyCloudFetchMinFreeMB        = "cloud.fetch_min_free_mb"
	KeyCloudFetchTimeoutSeconds   = "cloud.fetch_timeout_seconds"
	KeyCloudAllowChecksumOnly     = "cloud.allow_checksum_only"
	KeyCloudStateMaxKB            = "cloud.state_max_kb"
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
	Register(Definition{
		Key: KeyCloudFetchMaxMB, Category: CategoryDeploys, Kind: KindInt, Default: "256", Env: "APP_CLOUD_FETCH_MAX_MB", Apply: ApplyLive,
		CheckValue:  intRangeCheck(16, 1024),
		Title:       "OpenTofu download size cap",
		Description: "Largest OpenTofu archive or binary the control plane downloads, in MB from 16 to 1024. A larger download is refused. The next fetch uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudFetchMinFreeMB, Category: CategoryDeploys, Kind: KindInt, Default: "4096", Env: "APP_CLOUD_FETCH_MIN_FREE_MB", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, 1048576),
		Title:       "Free disk kept during the OpenTofu download",
		Description: "Free disk in MB that must remain after the OpenTofu download, from 0 to 1048576. The fetch is refused when the data disk has less. The next fetch uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudFetchTimeoutSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "300", Env: "APP_CLOUD_FETCH_TIMEOUT_SECONDS", Apply: ApplyLive,
		CheckValue:  intRangeCheck(10, 3600),
		Title:       "OpenTofu download timeout",
		Description: "Seconds the OpenTofu download may take, from 10 to 3600. The next fetch uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudAllowChecksumOnly, Category: CategorySecurity, Kind: KindBool, Default: "false", Env: "APP_CLOUD_ALLOW_CHECKSUM_ONLY", Apply: ApplyLive,
		Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Accept OpenTofu without a signature check",
		Description: "Run an OpenTofu download that matches the compiled checksum even when its release signature was not verified. Off refuses it. The next fetch uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudStateMaxKB, Category: CategoryDeploys, Kind: KindInt, Default: "1024", Env: "APP_CLOUD_STATE_MAX_KB", Apply: ApplyLive,
		CheckValue:  intRangeCheck(64, 65536),
		Title:       "Cloud state size cap",
		Description: "Largest cloud state one stack may store, in KB from 64 to 65536. A larger state is refused. The next save uses a changed value.",
	})
}
