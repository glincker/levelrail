package runtimesettings

// Registered keys for the AWS create guard and the per service enable flags.
const (
	KeyCloudGuardECR           = "cloud.guard.service.ecr"
	KeyCloudGuardACM           = "cloud.guard.service.acm"
	KeyCloudGuardParams        = "cloud.guard.service.parameters"
	KeyCloudGuardCloudTrail    = "cloud.guard.service.cloudtrail"
	KeyCloudGuardRDS           = "cloud.guard.service.rds"
	KeyCloudGuardElastiCache   = "cloud.guard.service.elasticache"
	KeyCloudGuardEBS           = "cloud.guard.service.ebs"
	KeyCloudGuardCloudWatch    = "cloud.guard.service.cloudwatch"
	KeyCloudGuardIdleCPU       = "cloud.guard.idle.cpu_percent"
	KeyCloudGuardIdleDays      = "cloud.guard.idle.days"
	KeyCloudGuardIdleEBSCents  = "cloud.guard.idle.ebs_gb_month_cents"
	KeyCloudGuardIdleEIPCents  = "cloud.guard.idle.eip_month_cents"
	KeyCloudGuardIdleALBCents  = "cloud.guard.idle.alb_month_cents"
	KeyCloudGuardKeepDaily     = "cloud.guard.snapshot.keep_daily"
	KeyCloudGuardKeepWeekly    = "cloud.guard.snapshot.keep_weekly"
	KeyCloudGuardMonthlyCapUSD = "cloud.guard.monthly_cap_usd"
	KeyCloudGuardAccountCaps   = "cloud.guard.account_caps"
	KeyCloudGuardPerCreateUSD  = "cloud.guard.per_create_max_usd"
	KeyCloudGuardInstanceTypes = "cloud.guard.instance_types"
	KeyCloudGuardKillSwitch    = "cloud.guard.kill_switch"
	KeyCloudGuardExpiresHours  = "cloud.guard.default_expires_hours"
	KeyCloudGuardPolicyARN     = "cloud.guard.policy_arn"
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
	flag(KeyCloudGuardRDS, "APP_CLOUD_GUARD_RDS", "AWS databases (RDS and Aurora)", "Lets the control plane list RDS instances and clusters, snapshot, stop, start and adopt them, and create managed ones behind the guard.")
	flag(KeyCloudGuardEBS, "APP_CLOUD_GUARD_EBS", "AWS disk snapshots (EBS)", "Lets the control plane list EBS volumes and snapshots, create managed snapshots and delete the ones it created.")
	flag(KeyCloudGuardCloudWatch, "APP_CLOUD_GUARD_CLOUDWATCH", "AWS metrics and logs (CloudWatch)", "Lets the control plane read CloudWatch metrics and logs and run idle detection. Each metric read is billed by AWS.")
	registerLimitInt(KeyCloudGuardIdleCPU, "APP_CLOUD_GUARD_IDLE_CPU_PERCENT", "5", "AWS idle CPU threshold (percent)", "An instance whose daily average CPU stays under this for the idle window is suggested for stopping. From 1 to 100.", 1, 100)
	registerLimitInt(KeyCloudGuardIdleDays, "APP_CLOUD_GUARD_IDLE_DAYS", "7", "AWS idle window (days)", "Days a resource must look idle before it is suggested for cleanup. From 1 to 90.", 1, 90)
	registerLimitInt(KeyCloudGuardIdleEBSCents, "APP_CLOUD_GUARD_IDLE_EBS_GB_MONTH_CENTS", "8", "AWS disk price for savings (cents per GB month)", "Unit price used to estimate the saving of an unattached volume. From 0 to 10000.", 0, 10000)
	registerLimitInt(KeyCloudGuardIdleEIPCents, "APP_CLOUD_GUARD_IDLE_EIP_MONTH_CENTS", "365", "AWS Elastic IP price for savings (cents per month)", "Unit price used to estimate the saving of an unassociated address. From 0 to 100000.", 0, 100000)
	registerLimitInt(KeyCloudGuardIdleALBCents, "APP_CLOUD_GUARD_IDLE_ALB_MONTH_CENTS", "1643", "AWS load balancer price for savings (cents per month)", "Unit price used to estimate the saving of an idle application load balancer. From 0 to 1000000.", 0, 1000000)
	registerLimitInt(KeyCloudGuardKeepDaily, "APP_CLOUD_GUARD_SNAPSHOT_KEEP_DAILY", "7", "AWS snapshots to keep (daily)", "Managed snapshots per volume kept for the most recent days when planning cleanup. 0 for both settings keeps everything.", 0, 365)
	registerLimitInt(KeyCloudGuardKeepWeekly, "APP_CLOUD_GUARD_SNAPSHOT_KEEP_WEEKLY", "4", "AWS snapshots to keep (weekly)", "Managed snapshots per volume kept for the most recent weeks when planning cleanup.", 0, 520)
	flag(KeyCloudGuardElastiCache, "APP_CLOUD_GUARD_ELASTICACHE", "AWS caches (ElastiCache)", "Lets the control plane list ElastiCache replication groups and clusters, snapshot and reboot managed ones, and delete them with a typed confirm.")
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
	Register(Definition{
		Key: KeyCloudGuardPolicyARN, Category: cat, Kind: KindString, Default: "", Env: "APP_CLOUD_GUARD_POLICY_ARN", Apply: ApplyLive,
		Title: "AWS guard policy ARN", Description: "Optional IAM managed deny policy, for example a budget guard. The AWS link kit attaches it to the role when the request names none.",
	})
}
