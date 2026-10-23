package functions

import (
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment variables that size the per-function key-value store.
const (
	EnvKVMaxKeyBytes   = "APP_FUNCTION_KV_MAX_KEY_BYTES"
	EnvKVMaxValueBytes = "APP_FUNCTION_KV_MAX_VALUE_BYTES"
	EnvKVMaxKeys       = "APP_FUNCTION_KV_MAX_KEYS"
	EnvKVMaxTotalBytes = "APP_FUNCTION_KV_MAX_TOTAL_BYTES"
	EnvKVMaxTTL        = "APP_FUNCTION_KV_MAX_TTL_SECONDS"
	EnvKVURL           = "APP_FUNCTION_KV_URL"

	// KVTokenEnv and KVURLEnv are the variables the function sees.
	KVTokenEnv = "KV_TOKEN"
	KVURLEnv   = "KV_URL"

	// KVAPIPath is where functions reach the store, below the control plane's internal address.
	KVAPIPath = "/api/v1/function-kv"
)

const (
	defaultKVMaxKeyBytes   = 512
	defaultKVMaxValueBytes = 256 * 1024
	defaultKVMaxKeys       = 10000
	defaultKVMaxTotalBytes = 64 * 1024 * 1024
	defaultKVMaxTTL        = 365 * 24 * 3600
)

// KVConfig is the server-wide key-value store sizing.
type KVConfig struct {
	store.KVLimits
	MaxTTLSeconds int
}

// KVConfigFromEnv reads KVConfig from the environment; getenv nil uses os.Getenv.
func KVConfigFromEnv(getenv func(string) string) KVConfig {
	if getenv == nil {
		getenv = os.Getenv
	}
	pick := func(name string, def int64) int64 {
		if n, err := strconv.ParseInt(getenv(name), 10, 64); err == nil && n > 0 {
			return n
		}
		return def
	}
	return KVConfig{
		KVLimits: store.KVLimits{
			MaxKeyBytes:   int(pick(EnvKVMaxKeyBytes, defaultKVMaxKeyBytes)),
			MaxValueBytes: int(pick(EnvKVMaxValueBytes, defaultKVMaxValueBytes)),
			MaxKeys:       int(pick(EnvKVMaxKeys, defaultKVMaxKeys)),
			MaxTotalBytes: pick(EnvKVMaxTotalBytes, defaultKVMaxTotalBytes),
		},
		MaxTTLSeconds: int(pick(EnvKVMaxTTL, defaultKVMaxTTL)),
	}
}
