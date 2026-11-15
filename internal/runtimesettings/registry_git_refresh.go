package runtimesettings

import "time"

// Git provider credential renewal: OAuth access tokens refresh themselves
// ahead of expiry, and a connection only alerts when a refresh keeps failing.
const (
	KeyGitRefreshEnabled      = "git.oauth_refresh_enabled"
	KeyGitRefreshLead         = "git.oauth_refresh_lead"
	KeyGitRefreshInterval     = "git.oauth_refresh_interval"
	KeyGitRefreshFailureGrace = "git.oauth_refresh_failure_grace"
	KeyGitRotateLifetimeDays  = "git.rotate_lifetime_days"

	gitRefreshLeadFloor       = time.Minute
	gitRefreshLeadMax         = 24 * time.Hour
	gitRefreshIntervalFloor   = 30 * time.Second
	gitRefreshIntervalMax     = 24 * time.Hour
	gitRefreshFailureGraceMax = 24 * time.Hour
	gitRotateLifetimeMax      = 400
)

func init() {
	Register(Definition{
		Key: KeyGitRefreshEnabled, Category: CategorySecurity, Kind: KindBool, Default: "true",
		Env: "APP_GIT_OAUTH_REFRESH_ENABLED", Apply: ApplyLive,
		Title:       "Git OAuth auto-renew",
		Description: "Refresh git provider OAuth access tokens in the background before they expire, so authorized connections keep working without a reminder. Turn it off to refresh only when a request needs the credential.",
	})
	Register(Definition{
		Key: KeyGitRefreshLead, Category: CategorySecurity, Kind: KindDuration, Default: "10m",
		Env: "APP_GIT_OAUTH_REFRESH_LEAD", Apply: ApplyLive, CheckValue: durationRangeCheck(gitRefreshLeadFloor, gitRefreshLeadMax),
		Title:       "Git OAuth refresh lead",
		Description: "How long before a git provider access token expires that the control plane refreshes it, from 1m to 24h.",
	})
	Register(Definition{
		Key: KeyGitRefreshInterval, Category: CategorySecurity, Kind: KindDuration, Default: "1m",
		Env: "APP_GIT_OAUTH_REFRESH_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(gitRefreshIntervalFloor, gitRefreshIntervalMax),
		Title:       "Git OAuth refresh check interval",
		Description: "How often the control plane looks for git provider access tokens that are due for a refresh, from 30s to 24h.",
	})
	Register(Definition{
		Key: KeyGitRefreshFailureGrace, Category: CategorySecurity, Kind: KindDuration, Default: "30m",
		Env: "APP_GIT_OAUTH_REFRESH_FAILURE_GRACE", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, gitRefreshFailureGraceMax),
		Title:       "Git OAuth refresh failure grace",
		Description: "How long a git provider can stay unreachable during a refresh before the connection raises an alert, from 1m to 24h. A revoked or expired authorization alerts at once.",
	})
	Register(Definition{
		Key: KeyGitRotateLifetimeDays, Category: CategorySecurity, Kind: KindInt, Default: "90",
		Env: "APP_GIT_ROTATE_LIFETIME_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(1, gitRotateLifetimeMax),
		Title:       "Git token rotation lifetime",
		Description: "Days a rotated git provider personal access token stays valid when the request names no expiry, from 1 to 400. The host may cap it lower.",
	})
}
