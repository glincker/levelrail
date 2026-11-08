package runtimesettings

// External secret provider (Doppler, Infisical, 1Password) read tunables.
const (
	KeySecretProviderTimeout     = "secrets.provider_timeout_seconds"   //nolint:gosec // setting key, not a credential
	KeySecretProviderCacheTTL    = "secrets.provider_cache_ttl_seconds" //nolint:gosec // setting key, not a credential
	KeySecretProviderMaxResponse = "secrets.provider_max_response_kib"  //nolint:gosec // setting key, not a credential
)

func init() {
	Register(Definition{
		Key: KeySecretProviderTimeout, Category: CategorySecurity, Kind: KindInt, Default: "10",
		Env: "APP_SECRET_PROVIDER_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 120),
		Title:       "Secret providers: request timeout",
		Description: "Seconds one read from Doppler, Infisical or 1Password may take, from 1 to 120. A read that runs longer fails the deploy with a timeout reason. The next read uses a changed value.",
	})
	Register(Definition{
		Key: KeySecretProviderCacheTTL, Category: CategorySecurity, Kind: KindInt, Default: "60",
		Env: "APP_SECRET_PROVIDER_CACHE_TTL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 3600),
		Title:       "Secret providers: cache time",
		Description: "Seconds a value read from Doppler, Infisical or 1Password is kept in memory and reused, from 0 to 3600. 0 reads the provider on every container start. Values are never written to disk.",
	})
	Register(Definition{
		Key: KeySecretProviderMaxResponse, Category: CategorySecurity, Kind: KindInt, Default: "1024",
		Env: "APP_SECRET_PROVIDER_MAX_RESPONSE_KIB", Apply: ApplyLive, CheckValue: intRangeCheck(16, 16384),
		Title:       "Secret providers: response size limit",
		Description: "Largest provider response read, in KiB, from 16 to 16384. A larger response fails the read instead of being parsed.",
	})
}
