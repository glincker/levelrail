package staticroutes

import (
	"strconv"
)

// Environment variables that tune the caps below. Every one is optional.
const (
	EnvMaxRules       = "APP_STATIC_ROUTES_MAX_RULES"
	EnvMaxHeaderRules = "APP_STATIC_ROUTES_MAX_HEADER_BLOCKS"
	EnvMaxFileBytes   = "APP_STATIC_ROUTES_MAX_FILE_BYTES"
	EnvMaxValueLen    = "APP_STATIC_ROUTES_MAX_VALUE_LEN"
)

// Limits caps the size of a site's routing rules.
type Limits struct {
	MaxRules        int
	MaxHeaderBlocks int
	MaxFileBytes    int
	MaxValueLen     int
}

// DefaultLimits are the caps used when no environment override is set.
func DefaultLimits() Limits {
	return Limits{MaxRules: 1000, MaxHeaderBlocks: 100, MaxFileBytes: 256 << 10, MaxValueLen: 2048}
}

// LimitsFromEnv applies any valid positive override found through lookup.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	l := DefaultLimits()
	set := func(key string, dst *int) {
		if lookup == nil {
			return
		}
		if v, ok := lookup(key); ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				*dst = n
			}
		}
	}
	set(EnvMaxRules, &l.MaxRules)
	set(EnvMaxHeaderRules, &l.MaxHeaderBlocks)
	set(EnvMaxFileBytes, &l.MaxFileBytes)
	set(EnvMaxValueLen, &l.MaxValueLen)
	return l
}
