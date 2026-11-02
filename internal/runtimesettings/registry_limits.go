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
	n := s.Int(ctx, key)
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
		Key: KeyAPIRateLimitRead, Category: CategorySecurity, Default: "600", Env: "APP_API_RATE_LIMIT_READ_RPM", Apply: ApplyLive,
		Title:       "API read rate limit",
		Description: "Read requests per minute allowed for each API token, signed-in user or client address. Requests over the budget get HTTP 429 with a Retry-After header. Minimum 60 so the dashboard keeps working; a change applies at once and keeps each caller's current budget.",
	}, 60, rateLimitCeiling)
	registerLimit(Definition{
		Key: KeyAPIRateLimitWrite, Category: CategorySecurity, Default: "120", Env: "APP_API_RATE_LIMIT_WRITE_RPM", Apply: ApplyLive,
		Title:       "API write rate limit",
		Description: "Write and deploy requests per minute allowed for each API token, signed-in user or client address. Requests over the budget get HTTP 429 with a Retry-After header. Minimum 20 so an operator cannot lock themselves out; a change applies at once.",
	}, 20, rateLimitCeiling)
	registerLimit(Definition{
		Key: KeyAPIRateLimitRedeem, Category: CategorySecurity, Default: "10", Env: "APP_API_RATE_LIMIT_TOKEN_REDEEM_RPM", Apply: ApplyLive,
		Title:       "Password reset and invite redeem rate limit",
		Description: "Password reset and invite accept requests per minute allowed from one client address. Requests over the budget get HTTP 429. Minimum 3; a change applies at once.",
	}, 3, rateLimitCeiling)
	registerLimit(Definition{
		Key: KeyAuthRateLimitPerIP, Category: CategorySecurity, Default: "60", Env: "APP_AUTH_ENGINE_RATE_LIMIT_PER_IP", Apply: ApplyRestart,
		Title:       "Sign-in rate limit per address",
		Description: "Credential requests (sign in, password reset, MFA) per minute allowed from one client address before it gets HTTP 429. This slows credential stuffing; it cannot be turned off and the minimum is 10 so an admin can still sign in. Needs a restart because the auth library holds the budget.",
	}, AuthRateLimitFloor, 100_000)
	registerLimit(Definition{
		Key: KeyAuthRateLimitPerEmail, Category: CategorySecurity, Default: "60", Env: "APP_AUTH_ENGINE_RATE_LIMIT_PER_EMAIL", Apply: ApplyRestart,
		Title:       "Sign-in rate limit per account",
		Description: "Credential requests per minute allowed for one account name from any address before it gets HTTP 429. This slows targeted password guessing; it cannot be turned off and the minimum is 10 so the owner can still sign in. Needs a restart because the auth library holds the budget.",
	}, AuthRateLimitFloor, 100_000)
	registerLimit(Definition{
		Key: KeyAPIMaxBodyBytes, Category: CategoryNetworking, Default: strconv.Itoa(16 << 20), Env: "APP_API_MAX_BODY_BYTES", Apply: ApplyLive,
		Title:       "API request body limit",
		Description: "Largest JSON request body in bytes the API reads before any handler runs; a bigger body is cut off and the request fails with HTTP 413. Source uploads, git pushes and asset uploads have their own limits. Between 1 MiB and 1 GiB; a change applies to the next request.",
	}, bodyLimitFloor, apiMaxBodyCeiling)
	registerLimit(Definition{
		Key: KeyAssetsUploadMaxBytes, Category: CategoryNetworking, Default: strconv.Itoa(1 << 30), Env: "APP_ASSETS_UPLOAD_MAX_BYTES", Apply: ApplyLive,
		Title:       "Asset upload size limit",
		Description: "Largest single asset object in bytes accepted by a direct or presigned upload; a bigger upload is refused with HTTP 413. Between 1 MiB and 1 TiB; a change applies to the next upload.",
	}, bodyLimitFloor, assetsUploadCeiling)
	registerLimit(Definition{
		Key: KeyDeployMaxConcurrent, Category: CategoryDeploys, Default: "0", Env: "APP_DEPLOY_MAX_CONCURRENT", Apply: ApplyLive,
		Title:       "Concurrent deploys",
		Description: "How many deploys may run at once across all apps; extra deploys wait in the queue and start when one finishes. 0 means no limit. A change applies to the next queue decision.",
	}, 0, deployMaxConcurrentCeiling)
}
