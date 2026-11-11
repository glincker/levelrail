package runtimesettings

// HashiCorp Vault and OpenBao client tunables.
const (
	KeyVaultCacheTTL        = "secrets.vault_cache_ttl_seconds"   //nolint:gosec // setting key, not a credential
	KeyVaultAllowPrivate    = "vault.allow_private_address"       //nolint:gosec // setting key, not a credential
	KeyVaultTokenRenewAfter = "secrets.vault_token_renew_percent" //nolint:gosec // setting key, not a credential
)

func init() {
	Register(Definition{
		Key: KeyVaultCacheTTL, Category: CategorySecurity, Kind: KindInt, Default: "30",
		Env: "APP_VAULT_CACHE_TTL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 3600),
		Title:       "Vault: secret cache time",
		Description: "Seconds a value read from Vault or OpenBao is kept in memory and reused, from 0 to 3600. 0 reads Vault on every container start. Values are never written to disk and are dropped when the connection or credential changes.",
	})
	Register(Definition{
		Key: KeyVaultAllowPrivate, Category: CategorySecurity, Kind: KindBool, Default: "true",
		Env: "APP_VAULT_ALLOW_PRIVATE_ADDRESS", Apply: ApplyLive,
		Title:       "Vault: allow private addresses",
		Description: "Allow the Vault address, which an admin sets, to resolve to a loopback or private network address, for a Vault or OpenBao on the same host or LAN. On by default; turn it off to refuse private addresses. Link-local, metadata, unspecified and multicast addresses are always blocked, and redirects to another host are never followed.",
	})
	Register(Definition{
		Key: KeyVaultTokenRenewAfter, Category: CategorySecurity, Kind: KindInt, Default: "80",
		Env: "APP_VAULT_TOKEN_RENEW_PERCENT", Apply: ApplyLive, CheckValue: intRangeCheck(10, 95),
		Title:       "Vault: login renewal point",
		Description: "Percent of a Vault login token lifetime after which Levelrail logs in again before the next read, from 10 to 95. A failed login fails the read; no expired token is reused.",
	})
}
