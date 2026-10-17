// Package autoscale is the deterministic replica autoscaler: a level-triggered
// controller that turns observed load into a desired replica count.
package autoscale

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// Metrics a policy can target.
const (
	MetricCPUPercent         = "cpu_percent"
	MetricMemoryPercent      = "memory_percent"
	MetricRequestsPerReplica = "requests_per_replica"
)

// Defaults applied by Normalize to unset policy fields.
const (
	DefaultScaleUpTolerancePct   = 10
	DefaultScaleDownTolerancePct = 30
	DefaultScaleUpCooldown       = 60 * time.Second
	DefaultScaleDownCooldown     = 10 * time.Minute
	DefaultStabilization         = 5 * time.Minute
	DefaultMaxStepUp             = 4
	DefaultMaxStepDown           = 1

	maxCooldownSeconds = 24 * 60 * 60
	maxStep            = 100
)

// Environment settings read by the control plane.
const (
	EnvMaxReplicasCeiling = "APP_AUTOSCALE_MAX_REPLICAS"
	EnvInterval           = "APP_AUTOSCALE_INTERVAL"
	EnvWindow             = "APP_AUTOSCALE_WINDOW"
	EnvMetricMaxAge       = "APP_AUTOSCALE_METRIC_MAX_AGE"
	EnvFlapThreshold      = "APP_AUTOSCALE_FLAP_THRESHOLD"

	DefaultMaxReplicasCeiling = 20
	DefaultInterval           = 30 * time.Second
	DefaultWindow             = 2 * time.Minute
	DefaultMetricMaxAge       = 90 * time.Second
	DefaultFlapThreshold      = 3
)

// ErrNotFound means an app has no autoscale policy.
var ErrNotFound = errors.New("autoscale: no policy for this app")

// Policy is one app's autoscaling contract.
type Policy struct {
	App         string  `json:"app"`
	MinReplicas int     `json:"min_replicas"`
	MaxReplicas int     `json:"max_replicas"`
	Metric      string  `json:"metric"`
	Target      float64 `json:"target"`
	// Scale up only when the metric is this far above Target, scale down only
	// when it is this far below: the gap between them is the dead band.
	ScaleUpTolerancePct   int `json:"scale_up_tolerance_pct"`
	ScaleDownTolerancePct int `json:"scale_down_tolerance_pct"`
	ScaleUpCooldownSecs   int `json:"scale_up_cooldown_secs"`
	ScaleDownCooldownSecs int `json:"scale_down_cooldown_secs"`
	StabilizationSecs     int `json:"stabilization_secs"`
	MaxStepUp             int `json:"max_step_up"`
	MaxStepDown           int `json:"max_step_down"`
}

// Normalize fills unset fields with their defaults. Hysteresis cannot be
// switched off: zero means the default, never "none".
func (p Policy) Normalize() Policy {
	if p.ScaleUpTolerancePct == 0 {
		p.ScaleUpTolerancePct = DefaultScaleUpTolerancePct
	}
	if p.ScaleDownTolerancePct == 0 {
		p.ScaleDownTolerancePct = DefaultScaleDownTolerancePct
	}
	if p.ScaleUpCooldownSecs == 0 {
		p.ScaleUpCooldownSecs = int(DefaultScaleUpCooldown / time.Second)
	}
	if p.ScaleDownCooldownSecs == 0 {
		p.ScaleDownCooldownSecs = int(DefaultScaleDownCooldown / time.Second)
	}
	if p.StabilizationSecs == 0 {
		p.StabilizationSecs = int(DefaultStabilization / time.Second)
	}
	if p.MaxStepUp == 0 {
		p.MaxStepUp = DefaultMaxStepUp
	}
	if p.MaxStepDown == 0 {
		p.MaxStepDown = DefaultMaxStepDown
	}
	return p
}

// Validate rejects a policy the controller could not act on safely. ceiling
// is the operator's global maximum (see CeilingFromEnv).
func (p Policy) Validate(ceiling int) error {
	switch {
	case p.App == "":
		return errors.New("app is required")
	case p.MinReplicas < 1:
		return errors.New("min_replicas must be at least 1")
	case p.MaxReplicas < 1:
		return errors.New("max_replicas is required")
	case p.MaxReplicas < p.MinReplicas:
		return errors.New("max_replicas must not be below min_replicas")
	case ceiling > 0 && p.MaxReplicas > ceiling:
		return fmt.Errorf("max_replicas must not exceed %d, the limit set by %s", ceiling, EnvMaxReplicasCeiling)
	}
	if err := p.validateTarget(); err != nil {
		return err
	}
	switch {
	case p.ScaleUpTolerancePct < 1 || p.ScaleUpTolerancePct > 100:
		return errors.New("scale_up_tolerance_pct must be between 1 and 100")
	case p.ScaleDownTolerancePct < 1 || p.ScaleDownTolerancePct > 99:
		return errors.New("scale_down_tolerance_pct must be between 1 and 99")
	case p.ScaleUpCooldownSecs < 1 || p.ScaleUpCooldownSecs > maxCooldownSeconds:
		return errors.New("scale_up_cooldown_secs must be between 1 and 86400")
	case p.ScaleDownCooldownSecs < 1 || p.ScaleDownCooldownSecs > maxCooldownSeconds:
		return errors.New("scale_down_cooldown_secs must be between 1 and 86400")
	case p.StabilizationSecs < 1 || p.StabilizationSecs > maxCooldownSeconds:
		return errors.New("stabilization_secs must be between 1 and 86400")
	case p.MaxStepUp < 1 || p.MaxStepUp > maxStep:
		return fmt.Errorf("max_step_up must be between 1 and %d", maxStep)
	case p.MaxStepDown < 1 || p.MaxStepDown > maxStep:
		return fmt.Errorf("max_step_down must be between 1 and %d", maxStep)
	}
	return nil
}

func (p Policy) validateTarget() error {
	if math.IsNaN(p.Target) || math.IsInf(p.Target, 0) || p.Target <= 0 {
		return errors.New("target must be greater than zero")
	}
	switch p.Metric {
	case MetricCPUPercent, MetricMemoryPercent:
		if p.Target > 100 {
			return fmt.Errorf("target for %s must be between 0 and 100", p.Metric)
		}
	case MetricRequestsPerReplica:
	default:
		return fmt.Errorf("metric must be %s, %s or %s", MetricCPUPercent, MetricMemoryPercent, MetricRequestsPerReplica)
	}
	return nil
}

// ScaleUpCooldown is the minimum gap between two scale ups.
func (p Policy) ScaleUpCooldown() time.Duration {
	return time.Duration(p.ScaleUpCooldownSecs) * time.Second
}

// ScaleDownCooldown is the minimum gap between a scale down and the last change.
func (p Policy) ScaleDownCooldown() time.Duration {
	return time.Duration(p.ScaleDownCooldownSecs) * time.Second
}

// Stabilization is how long a lower count must keep being recommended before it is applied.
func (p Policy) Stabilization() time.Duration {
	return time.Duration(p.StabilizationSecs) * time.Second
}

// CeilingFromEnv is the global maximum any policy may ask for.
func CeilingFromEnv(lookup func(string) string) int {
	return envInt(lookup, EnvMaxReplicasCeiling, DefaultMaxReplicasCeiling)
}

// IntervalFromEnv is how often the controller re-evaluates every policy.
func IntervalFromEnv(lookup func(string) string) time.Duration {
	return envDuration(lookup, EnvInterval, DefaultInterval)
}

// WindowFromEnv is the trailing window metrics are averaged over.
func WindowFromEnv(lookup func(string) string) time.Duration {
	return envDuration(lookup, EnvWindow, DefaultWindow)
}

// MetricMaxAgeFromEnv is how old the newest sample may be before the metric counts as unknown.
func MetricMaxAgeFromEnv(lookup func(string) string) time.Duration {
	return envDuration(lookup, EnvMetricMaxAge, DefaultMetricMaxAge)
}

// FlapThresholdFromEnv is the number of direction reversals in an hour that counts as flapping.
func FlapThresholdFromEnv(lookup func(string) string) int {
	return envInt(lookup, EnvFlapThreshold, DefaultFlapThreshold)
}

// OSEnv reads the process environment.
func OSEnv(key string) string { return os.Getenv(key) }

func envInt(lookup func(string) string, key string, def int) int {
	if n, err := strconv.Atoi(lookup(key)); err == nil && n > 0 {
		return n
	}
	return def
}

func envDuration(lookup func(string) string, key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(lookup(key)); err == nil && d > 0 {
		return d
	}
	return def
}
