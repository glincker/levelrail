package runtimesettings

// Registered keys for the AWS create guard and the per service enable flags.
const (
	KeyCloudGuardECR           = "cloud.guard.service.ecr"
	KeyCloudGuardACM           = "cloud.guard.service.acm"
	KeyCloudGuardParams        = "cloud.guard.service.parameters"
	KeyCloudGuardCloudTrail    = "cloud.guard.service.cloudtrail"
	KeyCloudGuardMonthlyCapUSD = "cloud.guard.monthly_cap_usd"
	KeyCloudGuardAccountCaps   = "cloud.guard.account_caps"
	KeyCloudGuardPerCreateUSD  = "cloud.guard.per_create_max_usd"
	KeyCloudGuardInstanceTypes = "cloud.guard.instance_types"
	KeyCloudGuardKillSwitch    = "cloud.guard.kill_switch"
	KeyCloudGuardExpiresHours  = "cloud.guard.default_expires_hours"
	cloudGuardMaxUSD           = 1_000_000
	cloudGuardMaxExpiresHours  = 24 * 365
)

func init() {
	cat := CategoryObservability
	flag := func(key, env, title, desc string) {
		Register(Definition{Key: key, Category: cat, Kind: KindBool, Default: "false", Env: env, Apply: ApplyLive, Title: title, Description: desc})
	}
	flag(KeyCloudGuardECR, "APP_CLOUD_GUARD_ECR", "AWS container registry", "Lets the control plane list and create ECR repositories and issue registry login tokens.")
	flag(KeyCloudGuardACM, "APP_CLOUD_GUARD_ACM", "AWS certificates", "Lets the control plane request ACM certificates.")
	flag(KeyCloudGuardParams, "APP_CLOUD_GUARD_PARAMS", "AWS parameter store", "Lets the control plane list and write SSM parameters and list Secrets Manager entry names.")
	flag(KeyCloudGuardCloudTrail, "APP_CLOUD_GUARD_CLOUDTRAIL", "AWS audit events", "Lets the control plane read CloudTrail events for a resource.")
	registerLimitInt(KeyCloudGuardMonthlyCapUSD, "APP_CLOUD_GUARD_MONTHLY_CAP_USD", "0", "AWS monthly cap (USD)", "Default monthly spend cap per account that blocks new creates. 0 means no cap.", 0, cloudGuardMaxUSD)
	Register(Definition{
		Key: KeyCloudGuardAccountCaps, Category: cat, Kind: KindStringList, Default: "", Env: "APP_CLOUD_GUARD_ACCOUNT_CAPS", Apply: ApplyLive,
		Title: "AWS cap per account", Description: "Entries like account-id=500 that override the default monthly cap for one account.",
	})
	registerLimitInt(KeyCloudGuardPerCreateUSD, "APP_CLOUD_GUARD_PER_CREATE_MAX_USD", "0", "AWS per create limit (USD)", "Largest estimated monthly cost one create may have. 0 means no limit.", 0, cloudGuardMaxUSD)
	Register(Definition{
		Key: KeyCloudGuardInstanceTypes, Category: cat, Kind: KindStringList, Default: "", Env: "APP_CLOUD_GUARD_INSTANCE_TYPES", Apply: ApplyLive,
		Title: "AWS instance type allowlist", Description: "Instance types a create may use. Empty refuses every create that names a type.",
	})
	Register(Definition{
		Key: KeyCloudGuardKillSwitch, Category: cat, Kind: KindBool, Default: "false", Env: "APP_CLOUD_GUARD_KILL_SWITCH", Apply: ApplyLive,
		Title: "AWS kill switch", Description: "Pauses every AWS create at once. Reads and existing resources are untouched.",
	})
	registerLimitInt(KeyCloudGuardExpiresHours, "APP_CLOUD_GUARD_DEFAULT_EXPIRES_HOURS", "168", "AWS default expiry (hours)", "Hours until a new resource is marked expired in its tags. 0 omits the tag.", 0, cloudGuardMaxExpiresHours)
}
