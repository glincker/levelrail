package runtimesettings

import (
	"context"
	"strconv"
)

// Registered keys for request limits and deploy concurrency.
const (
	KeyAPIRateLimitRead        = "api.rate_limit_read_rpm"
	KeyAPIRateLimitWrite       = "api.rate_limit_write_rpm"
	KeyAPIRateLimitRedeem      = "api.rate_limit_redeem_rpm"
	KeyAuthRateLimitPerIP      = "auth.engine_rate_limit_per_ip"
	KeyAuthRateLimitPerEmail   = "auth.engine_rate_limit_per_email"
	KeyAPIMaxBodyBytes         = "api.max_body_bytes"
	KeyAssetsUploadMaxBytes    = "assets.upload_max_bytes"
	KeyDeployMaxConcurrent     = "deploy.max_concurrent"
	KeyInlineSourceMaxBytes    = "deploy.inline_source_max_bytes"
	KeyInlineSourceMaxFiles    = "deploy.inline_source_max_files"
	KeyInlineSourceKeep        = "deploy.inline_source_keep"
	inlineSourceKeepCeiling    = 1000
	inlineSourceBytesFloor     = 1 << 10
	inlineSourceBytesCeiling   = 8 << 20
	inlineSourceFilesCeiling   = 200
	rateLimitCeiling           = 10_000_000
	bodyLimitFloor             = 1 << 20
	apiMaxBodyCeiling          = 1 << 30
	assetsUploadCeiling        = 1 << 40
	deployMaxConcurrentCeiling = 1000
)

// AuthRateLimitFloor keeps the credential limiter high enough for an admin to sign in, MFA included, and low enough to stop stuffing.
const AuthRateLimitFloor = 10

type intLimit struct {
	lo, hi int
	def    int
}

var limitRanges = map[string]intLimit{}

func registerLimit(d Definition, lo, hi int) {
	def, err := strconv.Atoi(d.Default)
	if err != nil {
		panic("runtimesettings: limit default of " + d.Key + " is not an integer")
	}
	d.Kind = KindInt
	d.CheckValue = intRangeCheck(lo, hi)
	limitRanges[d.Key] = intLimit{lo: lo, hi: hi, def: def}
	Register(d)
}

// Limit returns an int limit setting forced into its safe range. API writes are already range-checked;
// this also covers env seeds, which are only parsed, so a stray 0 or huge value can never disable a limit.
func (s *Service) Limit(ctx context.Context, key string) int {
	return ClampLimit(key, s.Int(ctx, key))
}

// ClampLimit forces n into the safe range of a limit setting; boot code uses it so nothing starts below a floor.
func ClampLimit(key string, n int) int {
	r, ok := limitRanges[key]
	if !ok {
		return n
	}
	switch {
	case n < r.lo && n <= 0 && r.lo > 0:
		return r.def
	case n < r.lo:
		return r.lo
	case n > r.hi:
		return r.hi
	}
	return n
}

func init() {
	registerLimit(Definition{
		Key: KeyAPIRateLimitRead, Category: CategorySecurity, Default: "600", Env: "APP_API_RATE_LIMIT_READ_RPM", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "API read rate limit",
		Description: "Read requests per minute allowed for each API token, signed-in user or client address. Requests over the budget get HTTP 429 with a Retry-After header. Minimum 60 so the dashboard keeps working; a change applies at once and keeps each caller's current budget. A limit cannot be switched off: an environment value of 0 or below the minimum is raised to the minimum (older releases treated 0 as off).",
	}, 60, rateLimitCeiling)
	registerLimit(Definition{
		Key: KeyAPIRateLimitWrite, Category: CategorySecurity, Default: "120", Env: "APP_API_RATE_LIMIT_WRITE_RPM", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "API write rate limit",
		Description: "Write and deploy requests per minute allowed for each API token, signed-in user or client address. Requests over the budget get HTTP 429 with a Retry-After header. Minimum 20 so an operator cannot lock themselves out; a change applies at once. A limit cannot be switched off: an environment value of 0 or below the minimum is raised to the minimum (older releases treated 0 as off).",
	}, 20, rateLimitCeiling)
	registerLimit(Definition{
		Key: KeyAPIRateLimitRedeem, Category: CategorySecurity, Default: "10", Env: "APP_API_RATE_LIMIT_TOKEN_REDEEM_RPM", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Password reset and invite redeem rate limit",
		Description: "Password reset and invite accept requests per minute allowed from one client address. Requests over the budget get HTTP 429. Minimum 3; a change applies at once. A limit cannot be switched off: an environment value of 0 or below the minimum is raised to the minimum (older releases treated 0 as off).",
	}, 3, rateLimitCeiling)
	registerLimit(Definition{
		Key: KeyAuthRateLimitPerIP, Category: CategorySecurity, Default: "60", Env: "APP_AUTH_ENGINE_RATE_LIMIT_PER_IP", Apply: ApplyRestart, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Sign-in rate limit per address",
		Description: "Credential requests (sign in, password reset, MFA) per minute allowed from one client address before it gets HTTP 429. This slows credential stuffing; it cannot be turned off and the minimum is 10 so an admin can still sign in. An environment value below 10 is raised to 10. Needs a restart because the auth library holds the budget.",
	}, AuthRateLimitFloor, 100_000)
	registerLimit(Definition{
		Key: KeyAuthRateLimitPerEmail, Category: CategorySecurity, Default: "60", Env: "APP_AUTH_ENGINE_RATE_LIMIT_PER_EMAIL", Apply: ApplyRestart, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Sign-in rate limit per account",
		Description: "Credential requests per minute allowed for one account name from any address before it gets HTTP 429. This slows targeted password guessing; it cannot be turned off and the minimum is 10 so the owner can still sign in. An environment value below 10 is raised to 10. Needs a restart because the auth library holds the budget.",
	}, AuthRateLimitFloor, 100_000)
	registerLimit(Definition{
		Key: KeyAPIMaxBodyBytes, Category: CategoryNetworking, Default: strconv.Itoa(16 << 20), Env: "APP_API_MAX_BODY_BYTES", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "API request body limit",
		Description: "Largest JSON request body in bytes the API reads before any handler runs; a bigger body is cut off and the request fails with HTTP 413. Source uploads, git pushes and asset uploads have their own limits. Between 1 MiB and 1 GiB; a change applies to the next request.",
	}, bodyLimitFloor, apiMaxBodyCeiling)
	registerLimit(Definition{
		Key: KeyAssetsUploadMaxBytes, Category: CategoryNetworking, Default: strconv.Itoa(1 << 30), Env: "APP_ASSETS_UPLOAD_MAX_BYTES", Apply: ApplyLive,
		Title:       "Asset upload size limit",
		Description: "Largest single asset object in bytes accepted by a direct or presigned upload; a bigger upload is refused with HTTP 413. Between 1 MiB and 1 TiB; direct uploads are exempt from the API request body limit and obey only this one. A change applies to the next upload.",
	}, bodyLimitFloor, assetsUploadCeiling)
	registerLimit(Definition{
		Key: KeyDeployMaxConcurrent, Category: CategoryDeploys, Default: "0", Env: "APP_DEPLOY_MAX_CONCURRENT", Apply: ApplyLive,
		Title:       "Concurrent deploys",
		Description: "How many deploys may run at once across all apps; extra deploys wait in the queue and start when one finishes. 0 means no limit. A change applies to the next queue decision.",
	}, 0, deployMaxConcurrentCeiling)
	registerLimit(Definition{
		Key: KeyInlineSourceMaxBytes, Category: CategoryDeploys, Default: strconv.Itoa(256 << 10), Env: "APP_INLINE_SOURCE_MAX_BYTES", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Inline Dockerfile size limit",
		Description: "Largest pasted Dockerfile plus extra context files, in bytes, accepted by a deploy without a git repository; a bigger one is refused with HTTP 413. Between 1 KiB and 8 MiB, and never more than the API request body limit. A change applies to the next deploy.",
	}, inlineSourceBytesFloor, inlineSourceBytesCeiling)
	registerLimit(Definition{
		Key: KeyInlineSourceMaxFiles, Category: CategoryDeploys, Default: "20", Env: "APP_INLINE_SOURCE_MAX_FILES", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Inline Dockerfile extra files",
		Description: "How many extra context files may be sent next to a pasted Dockerfile; more are refused with HTTP 413. 0 allows the Dockerfile alone. A change applies to the next deploy.",
	}, 0, inlineSourceFilesCeiling)
	registerLimit(Definition{
		Key: KeyInlineSourceKeep, Category: CategoryDeploys, Default: "10", Env: "APP_INLINE_SOURCE_KEEP", Apply: ApplyLive,
		Title:       "Inline Dockerfiles kept per app",
		Description: "How many distinct pasted Dockerfiles each app keeps for redeploys; the hourly sweep deletes older ones unless a queued, held or running deploy still needs them. Between 1 and 1000. A change applies to the next sweep.",
	}, 1, inlineSourceKeepCeiling)
}

// KeyAppsMaxReplicasPerService caps the replica count one service may be given.
const KeyAppsMaxReplicasPerService = "apps.max_replicas_per_service"

func init() {
	registerLimit(Definition{
		Key: KeyAppsMaxReplicasPerService, Category: CategoryDeploys, Default: "100", Env: "APP_MAX_REPLICAS_PER_SERVICE", Apply: ApplyLive,
		Title:       "Replica cap per service",
		Description: "Largest replica count the API accepts for one service when an app is created, updated or scaled; a bigger count is refused with HTTP 400. Between 1 and 1000; a change applies to the next request, running apps keep their count.",
	}, 1, 1000)
}
