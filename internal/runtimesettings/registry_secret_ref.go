package runtimesettings

// Length limits for secret:// references and connection names.
const (
	KeySecretRefMaxRef      = "secrets.ref_max_ref_bytes"  //nolint:gosec // setting key, not a credential
	KeySecretRefMaxConn     = "secrets.ref_max_conn_bytes" //nolint:gosec // setting key, not a credential
	KeySecretRefMaxPath     = "secrets.ref_max_path_bytes" //nolint:gosec // setting key, not a credential
	KeySecretRefMaxSegments = "secrets.ref_max_path_depth" //nolint:gosec // setting key, not a credential
	KeySecretRefMaxKey      = "secrets.ref_max_key_bytes"  //nolint:gosec // setting key, not a credential
)

func init() {
	Register(Definition{
		Key: KeySecretRefMaxRef, Category: CategorySecurity, Kind: KindInt, Default: "1024",
		Env: "APP_SECRETREF_MAX_REF", Apply: ApplyLive, CheckValue: intRangeCheck(64, 8192),
		Title:       "Secret references: total length",
		Description: "Longest secret:// reference accepted, in bytes, from 64 to 8192. Longer references are rejected when written, not truncated.",
	})
	Register(Definition{
		Key: KeySecretRefMaxConn, Category: CategorySecurity, Kind: KindInt, Default: "63",
		Env: "APP_SECRETREF_MAX_CONN", Apply: ApplyLive, CheckValue: intRangeCheck(8, 128),
		Title:       "Secret references: connection name length",
		Description: "Longest secret connection name accepted, in bytes, from 8 to 128. Existing longer names keep working for reads but cannot be created again.",
	})
	Register(Definition{
		Key: KeySecretRefMaxPath, Category: CategorySecurity, Kind: KindInt, Default: "512",
		Env: "APP_SECRETREF_MAX_PATH", Apply: ApplyLive, CheckValue: intRangeCheck(16, 4096),
		Title:       "Secret references: path length",
		Description: "Longest path inside a secret reference, in bytes, from 16 to 4096.",
	})
	Register(Definition{
		Key: KeySecretRefMaxSegments, Category: CategorySecurity, Kind: KindInt, Default: "16",
		Env: "APP_SECRETREF_MAX_SEGMENTS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 64),
		Title:       "Secret references: path depth",
		Description: "Most slash separated segments a secret reference path may have, from 1 to 64.",
	})
	Register(Definition{
		Key: KeySecretRefMaxKey, Category: CategorySecurity, Kind: KindInt, Default: "128",
		Env: "APP_SECRETREF_MAX_KEY", Apply: ApplyLive, CheckValue: intRangeCheck(8, 512),
		Title:       "Secret references: key length",
		Description: "Longest key name inside a secret reference, in bytes, from 8 to 512.",
	})
}
