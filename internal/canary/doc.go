// Package canary decides when a staged canary release advances, promotes or
// rolls back, and runs the level-triggered reconciler that applies it.
package canary

import "github.com/GLINCKER/levelrail/internal/canarypolicy"

type (
	// Policy is the stage schedule and thresholds of one canary.
	Policy = canarypolicy.Policy
	// Limits are the global judgement knobs.
	Limits = canarypolicy.Limits
)

const (
	// MaxWeight is the largest share ingress gives a canary.
	MaxWeight = canarypolicy.MaxWeight
	// Full is the stage value that means promote.
	Full = canarypolicy.Full
)

var (
	// DefaultPolicy returns the env-tuned default policy.
	DefaultPolicy = canarypolicy.DefaultPolicy
	// DefaultLimits returns the env-tuned global limits.
	DefaultLimits = canarypolicy.DefaultLimits
	// ParseStages parses a comma separated stage list.
	ParseStages = canarypolicy.ParseStages
	// ValidateStages checks a stage schedule.
	ValidateStages = canarypolicy.ValidateStages
	// OSEnv reads the process environment.
	OSEnv = canarypolicy.OSEnv
)

// The APP_CANARY_* variable names.
const (
	EnvStages       = canarypolicy.EnvStages
	EnvInterval     = canarypolicy.EnvInterval
	EnvMaxErrorRate = canarypolicy.EnvMaxErrorRate
	EnvMaxP95Ratio  = canarypolicy.EnvMaxP95Ratio
	EnvMinRequests  = canarypolicy.EnvMinRequests
	EnvP95FloorMS   = canarypolicy.EnvP95FloorMS
	EnvTick         = canarypolicy.EnvTick
)
