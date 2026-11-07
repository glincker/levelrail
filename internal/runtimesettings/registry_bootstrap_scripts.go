package runtimesettings

import (
	"fmt"
	"strconv"
)

// Registered keys for the saved bootstrap script limits.
const (
	KeyBootstrapScriptMaxBytes = "bootstrap_scripts.max_bytes"
	KeyBootstrapScriptMaxCount = "bootstrap_scripts.max_count"

	DefaultBootstrapScriptMaxBytes = 64 << 10
	DefaultBootstrapScriptMaxCount = 50

	bootstrapScriptBytesCeiling = 8 << 20
	bootstrapScriptCountCeiling = 10_000
)

func boundedPositive(name string, hi int) func(string) error {
	return func(canonical string) error {
		var n int
		if _, err := fmt.Sscanf(canonical, "%d", &n); err != nil || n < 1 || n > hi {
			return fmt.Errorf("%s must be between 1 and %d", name, hi)
		}
		return nil
	}
}

func init() {
	Register(Definition{
		Key: KeyBootstrapScriptMaxBytes, Category: CategoryDeploys, Kind: KindInt,
		Default: strconv.Itoa(DefaultBootstrapScriptMaxBytes), Env: "APP_BOOTSTRAP_SCRIPT_MAX_BYTES", Apply: ApplyLive,
		CheckValue:  boundedPositive("size", bootstrapScriptBytesCeiling),
		Title:       "Bootstrap script size limit",
		Description: "Largest saved bootstrap script body, in bytes. Read on every create and update; the environment variable only seeds the default.",
	})
	Register(Definition{
		Key: KeyBootstrapScriptMaxCount, Category: CategoryDeploys, Kind: KindInt,
		Default: strconv.Itoa(DefaultBootstrapScriptMaxCount), Env: "APP_BOOTSTRAP_SCRIPT_MAX_COUNT", Apply: ApplyLive,
		CheckValue:  boundedPositive("count", bootstrapScriptCountCeiling),
		Title:       "Bootstrap script count limit",
		Description: "How many bootstrap scripts can be saved at once. Read on every create; the environment variable only seeds the default.",
	})
}
