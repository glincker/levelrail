package runtimesettings

// Keys for the CloudFront and Systems Manager AWS modules.
const (
	KeyCloudGuardCloudFront   = "cloud.guard.service.cloudfront"
	KeyCloudGuardSSM          = "cloud.guard.service.ssm"
	KeyCloudGuardSSMRun       = "cloud.guard.service.ssm_run"
	KeyCloudFrontMaxPaths     = "cloud.cloudfront.max_invalidation_paths"
	KeyCloudSSMCommands       = "cloud.ssm.command_allowlist"
	KeyCloudSSMTimeoutSeconds = "cloud.ssm.command_timeout_seconds"
)

func init() {
	cat := CategoryObservability
	flag := func(key, env, title, desc string) {
		Register(Definition{Key: key, Category: cat, Kind: KindBool, Default: "false", Env: env, Apply: ApplyLive, Title: title, Description: desc})
	}
	flag(KeyCloudGuardCloudFront, "APP_CLOUD_GUARD_CLOUDFRONT", "AWS CDN (CloudFront)", "Lets the control plane list CloudFront distributions and invalidations and create invalidations.")
	flag(KeyCloudGuardSSM, "APP_CLOUD_GUARD_SSM", "AWS managed instances (Systems Manager)", "Lets the control plane read which instances the Systems Manager agent manages.")
	flag(KeyCloudGuardSSMRun, "APP_CLOUD_GUARD_SSM_RUN", "AWS remote commands (Systems Manager)", "Lets a root caller run allowlisted shell commands on one instance and open session tokens. Needs the command allowlist.")
	registerLimitInt(KeyCloudFrontMaxPaths, "APP_CLOUD_CLOUDFRONT_MAX_INVALIDATION_PATHS", "50", "CloudFront paths per invalidation", "Most paths one invalidation request may carry. From 1 to 3000.", 1, 3000)
	Register(Definition{
		Key: KeyCloudSSMCommands, Category: cat, Kind: KindStringList, Default: "", Env: "APP_CLOUD_SSM_COMMAND_ALLOWLIST", Apply: ApplyLive,
		Title: "Systems Manager command allowlist", Description: "Exact shell commands a remote run may use. Empty refuses every command.",
	})
	registerLimitInt(KeyCloudSSMTimeoutSeconds, "APP_CLOUD_SSM_COMMAND_TIMEOUT_SECONDS", "60", "Systems Manager command timeout (seconds)", "Seconds a remote command may run before AWS stops it. From 30 to 3600.", 30, 3600)
}
