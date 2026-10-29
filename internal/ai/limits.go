package ai

import (
	"os"
	"strconv"
	"time"
)

// Environment variables that tune the assistant harness.
const (
	EnvStepTimeout         = "APP_AI_STEP_TIMEOUT"
	EnvMaxSteps            = "APP_AI_MAX_STEPS"
	EnvToolOutputMaxBytes  = "APP_AI_TOOL_OUTPUT_MAX_BYTES"
	EnvDefaultContext      = "APP_AI_DEFAULT_CONTEXT_TOKENS"
	EnvMonthlyTokenBudget  = "APP_AI_MONTHLY_TOKEN_BUDGET" //nolint:gosec // an env var name, not a credential
	EnvModelsCacheTTL      = "APP_AI_MODELS_CACHE_TTL"
	EnvRetryMaxAttempts    = "APP_AI_RETRY_MAX_ATTEMPTS"
	EnvRetryBaseDelay      = "APP_AI_RETRY_BASE_DELAY"
	EnvRetryMaxDelay       = "APP_AI_RETRY_MAX_DELAY"
	EnvValidateTimeout     = "APP_AI_VALIDATE_TIMEOUT"
	EnvRepeatLimit         = "APP_AI_REPEAT_LIMIT"
	EnvEmptyLimit          = "APP_AI_EMPTY_RESULT_LIMIT"
	defaultStepTimeout     = 120 * time.Second
	defaultMaxSteps        = 12
	defaultToolOutputBytes = 32 * 1024
	defaultContextTokens   = 128000
	defaultModelsCacheTTL  = 10 * time.Minute
	defaultRetryAttempts   = 3
	defaultRetryBase       = time.Second
	defaultRetryMax        = 30 * time.Second
	defaultValidateTimeout = 10 * time.Second
	defaultRepeatLimit     = 3
	defaultEmptyLimit      = 3
)

// Limits are the harness bounds, resolved from the environment.
type Limits struct {
	StepTimeout        time.Duration
	MaxSteps           int
	ToolOutputMaxBytes int
	DefaultContext     int
	MonthlyTokenBudget int64
	RepeatLimit        int
	EmptyLimit         int
}

// LimitsFromEnv reads the harness limits; unset or invalid values use defaults,
// and a budget of 0 means unlimited.
func LimitsFromEnv() Limits {
	return Limits{
		StepTimeout:        envDuration(EnvStepTimeout, defaultStepTimeout),
		MaxSteps:           envInt(EnvMaxSteps, defaultMaxSteps),
		ToolOutputMaxBytes: envInt(EnvToolOutputMaxBytes, defaultToolOutputBytes),
		DefaultContext:     envInt(EnvDefaultContext, defaultContextTokens),
		MonthlyTokenBudget: int64(envInt(EnvMonthlyTokenBudget, 0)),
		RepeatLimit:        envInt(EnvRepeatLimit, defaultRepeatLimit),
		EmptyLimit:         envInt(EnvEmptyLimit, defaultEmptyLimit),
	}
}

// ModelsCacheTTL is how long a provider's model list is reused.
func ModelsCacheTTL() time.Duration { return envDuration(EnvModelsCacheTTL, defaultModelsCacheTTL) }

// ValidateTimeout bounds the key check on save and test.
func ValidateTimeout() time.Duration { return envDuration(EnvValidateTimeout, defaultValidateTimeout) }

// RetryPolicyFromEnv reads the retry knobs.
func RetryPolicyFromEnv() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: envInt(EnvRetryMaxAttempts, defaultRetryAttempts),
		BaseDelay:   envDuration(EnvRetryBaseDelay, defaultRetryBase),
		MaxDelay:    envDuration(EnvRetryMaxDelay, defaultRetryMax),
	}
}

func envInt(name string, def int) int {
	if v, ok := os.LookupEnv(name); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

func envDuration(name string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(name); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}
