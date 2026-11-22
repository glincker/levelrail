package runtimesettings

// Keys of the live AWS SSO tunables.
const (
	KeyCloudAWSSSORenewPercent     = "cloud.aws.sso.renew_percent"
	KeyCloudAWSSSOTokenSkew        = "cloud.aws.sso.token_skew_seconds" //nolint:gosec // setting key, not a credential
	KeyCloudAWSSSOMaxAccounts      = "cloud.aws.sso.max_accounts"
	KeyCloudAWSSSOMaxSessions      = "cloud.aws.sso.max_sessions"
	KeyCloudAWSSSOTimeout          = "cloud.aws.sso.timeout_seconds"
	KeyCloudAWSSSOMaxResponseBytes = "cloud.aws.sso.max_response_bytes"
)

func init() {
	Register(Definition{
		Key: KeyCloudAWSSSORenewPercent, Category: CategorySecurity, Kind: KindInt, Default: "80",
		Env: "APP_CLOUD_AWS_SSO_RENEW_PERCENT", Apply: ApplyLive, CheckValue: intRangeCheck(1, 99),
		Title:       "AWS SSO: role credential renew point",
		Description: "Percent of a role credential lifetime after which the next use reads new keys, from 1 to 99. The next use applies a changed value.",
	})
	Register(Definition{
		Key: KeyCloudAWSSSOTokenSkew, Category: CategorySecurity, Kind: KindInt, Default: "60",
		Env: "APP_CLOUD_AWS_SSO_TOKEN_SKEW_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 3600),
		Title:       "AWS SSO: access token skew",
		Description: "Seconds before an AWS SSO access token expires that it is refreshed, from 0 to 3600. The next use applies a changed value.",
	})
	Register(Definition{
		Key: KeyCloudAWSSSOMaxAccounts, Category: CategorySecurity, Kind: KindInt, Default: "100",
		Env: "APP_CLOUD_AWS_SSO_MAX_ACCOUNTS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1000),
		Title:       "AWS SSO: accounts listed at login",
		Description: "Most accounts read from IAM Identity Center when someone signs in, from 1 to 1000. The next login applies a changed value.",
	})
	Register(Definition{
		Key: KeyCloudAWSSSOMaxSessions, Category: CategorySecurity, Kind: KindInt, Default: "20",
		Env: "APP_CLOUD_AWS_SSO_MAX_SESSIONS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 500),
		Title:       "AWS SSO: sign in sessions at once",
		Description: "Most AWS SSO sign in sessions that may be open at the same time, from 1 to 500. The next sign in applies a changed value.",
	})
	Register(Definition{
		Key: KeyCloudAWSSSOTimeout, Category: CategorySecurity, Kind: KindInt, Default: "15",
		Env: "APP_CLOUD_AWS_SSO_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(2, 120),
		Title:       "AWS SSO: request timeout",
		Description: "Seconds one call to the IAM Identity Center sign in or portal endpoints may take, from 2 to 120. The next call uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudAWSSSOMaxResponseBytes, Category: CategorySecurity, Kind: KindInt, Default: "4194304",
		Env: "APP_CLOUD_AWS_MAX_RESPONSE_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(65536, 33554432),
		Title:       "AWS SSO: response size limit",
		Description: "Most bytes read from one IAM Identity Center response, from 65536 to 33554432. The next call uses a changed value.",
	})
}
