package deploysafety

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment variables that set the platform-wide defaults.
const (
	EnvSuccessThreshold = "APP_DEPLOY_SUCCESS_THRESHOLD"
	EnvStableWindow     = "APP_DEPLOY_STABLE_WINDOW"
	EnvDrainTimeout     = "APP_DEPLOY_DRAIN_TIMEOUT"
	EnvWatchWindow      = "APP_DEPLOY_WATCH_WINDOW"
	EnvWatchMaxRestarts = "APP_DEPLOY_WATCH_MAX_RESTARTS"
	EnvWatchErrorRate   = "APP_DEPLOY_WATCH_ERROR_RATE"
	EnvWatchMinRequests = "APP_DEPLOY_WATCH_MIN_REQUESTS"
	EnvAutoRollback     = "APP_DEPLOY_AUTO_ROLLBACK"
	EnvWatchMode        = "APP_DEPLOY_WATCH_MODE"
	EnvWatchLatency     = "APP_DEPLOY_WATCH_LATENCY_RATIO"
	EnvWatchLatencyMin  = "APP_DEPLOY_WATCH_LATENCY_MIN_DELTA"
	EnvWatchSoftTicks   = "APP_DEPLOY_WATCH_SOFT_TICKS"
	EnvWatchSoftRoll    = "APP_DEPLOY_WATCH_SOFT_ROLLBACK"
	EnvRollbackPins     = "APP_DEPLOY_ROLLBACK_PINS"
	EnvWatchInterval    = "APP_DEPLOY_WATCH_INTERVAL"
)

// Effective is a fully resolved set of gate and watch settings.
type Effective struct {
	// SuccessThreshold is how many consecutive readiness passes a new
	// container needs before it may take traffic.
	SuccessThreshold int `json:"success_threshold"`
	// StableWindow is how long a new container must stay running, without
	// a restart or OOM kill, before it may take traffic.
	StableWindow time.Duration `json:"stable_window"`
	// DrainTimeout is the minimum time the previous release keeps serving
	// after cutover, so in-flight requests finish before it stops.
	DrainTimeout     time.Duration `json:"drain_timeout"`
	WatchWindow      time.Duration `json:"watch_window"`
	WatchMaxRestarts int           `json:"watch_max_restarts"`
	WatchErrorRate   float64       `json:"watch_error_rate"`
	WatchMinRequests int           `json:"watch_min_requests"`
	// AutoRollback is derived from WatchMode (auto).
	AutoRollback bool `json:"auto_rollback"`
	// WatchMode is off, notify or auto.
	WatchMode string `json:"watch_mode"`
	// WatchLatencyRatio trips the soft latency signal when the new
	// release's p95 reaches this multiple of the previous release's; 0
	// turns the signal off.
	WatchLatencyRatio float64 `json:"watch_latency_ratio"`
	// WatchLatencyMinDelta is the absolute p95 increase also required, so
	// a 4ms to 9ms move is never a regression.
	WatchLatencyMinDelta time.Duration `json:"watch_latency_min_delta"`
	// WatchSoftTicks is how many consecutive evaluations a soft signal must
	// hold before it counts.
	WatchSoftTicks int `json:"watch_soft_ticks"`
	// WatchSoftRollback lets a soft signal roll back (mode auto only);
	// otherwise it only notifies.
	WatchSoftRollback bool `json:"watch_soft_rollback"`
	// RollbackPins is how many past releases per app stay pinned.
	RollbackPins  int           `json:"rollback_pins"`
	WatchInterval time.Duration `json:"watch_interval"`
}

// BuiltinDefaults are the defaults with no environment set.
func BuiltinDefaults() Effective {
	return Effective{
		SuccessThreshold:     2,
		StableWindow:         10 * time.Second,
		DrainTimeout:         15 * time.Second,
		WatchWindow:          5 * time.Minute,
		WatchMaxRestarts:     2,
		WatchErrorRate:       0.2,
		WatchMinRequests:     20,
		AutoRollback:         true,
		WatchMode:            WatchModeAuto,
		WatchLatencyRatio:    2,
		WatchLatencyMinDelta: 100 * time.Millisecond,
		WatchSoftTicks:       3,
		RollbackPins:         3,
		WatchInterval:        10 * time.Second,
	}
}

// DefaultsFromEnv reads the APP_DEPLOY_* defaults over BuiltinDefaults. An
// invalid value keeps the builtin and is reported in warnings.
func DefaultsFromEnv(lookup func(string) (string, bool)) (Effective, []error) {
	d := BuiltinDefaults()
	var warnings []error
	envInt := func(key string, dst *int, lowest int) {
		raw, ok := lookup(key)
		if !ok || raw == "" {
			return
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < lowest {
			warnings = append(warnings, fmt.Errorf("%s=%q: want an integer >= %d", key, raw, lowest))
			return
		}
		*dst = n
	}
	envDur := func(key string, dst *time.Duration) {
		raw, ok := lookup(key)
		if !ok || raw == "" {
			return
		}
		v, err := time.ParseDuration(raw)
		if err != nil || v < 0 {
			warnings = append(warnings, fmt.Errorf("%s=%q: want a non-negative duration", key, raw))
			return
		}
		*dst = v
	}
	envInt(EnvSuccessThreshold, &d.SuccessThreshold, 1)
	envDur(EnvStableWindow, &d.StableWindow)
	envDur(EnvDrainTimeout, &d.DrainTimeout)
	envDur(EnvWatchWindow, &d.WatchWindow)
	envInt(EnvWatchMaxRestarts, &d.WatchMaxRestarts, 1)
	envInt(EnvWatchMinRequests, &d.WatchMinRequests, 1)
	envInt(EnvRollbackPins, &d.RollbackPins, 0)
	envDur(EnvWatchInterval, &d.WatchInterval)
	if raw, ok := lookup(EnvWatchErrorRate); ok && raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v <= 0 || v > 1 {
			warnings = append(warnings, fmt.Errorf("%s=%q: want a fraction in (0, 1]", EnvWatchErrorRate, raw))
		} else {
			d.WatchErrorRate = v
		}
	}
	envDur(EnvWatchLatencyMin, &d.WatchLatencyMinDelta)
	envInt(EnvWatchSoftTicks, &d.WatchSoftTicks, 1)
	if raw, ok := lookup(EnvWatchLatency); ok && raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || (v != 0 && v < 1) {
			warnings = append(warnings, fmt.Errorf("%s=%q: want 0 (off) or a ratio >= 1", EnvWatchLatency, raw))
		} else {
			d.WatchLatencyRatio = v
		}
	}
	if raw, ok := lookup(EnvWatchSoftRoll); ok && raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s=%q: want a boolean", EnvWatchSoftRoll, raw))
		} else {
			d.WatchSoftRollback = v
		}
	}
	if raw, ok := lookup(EnvAutoRollback); ok && raw != "" {
		v, err := strconv.ParseBool(raw)
		switch {
		case err != nil:
			warnings = append(warnings, fmt.Errorf("%s=%q: want a boolean", EnvAutoRollback, raw))
		case v:
			d.WatchMode = WatchModeAuto
		default:
			d.WatchMode = WatchModeNotify
		}
	}
	if raw, ok := lookup(EnvWatchMode); ok && raw != "" {
		if ValidWatchMode(raw) {
			d.WatchMode = raw
		} else {
			warnings = append(warnings, fmt.Errorf("%s=%q: want off, notify or auto", EnvWatchMode, raw))
		}
	}
	d.AutoRollback = d.WatchMode == WatchModeAuto
	if d.WatchInterval <= 0 {
		d.WatchInterval = BuiltinDefaults().WatchInterval
	}
	return d, warnings
}

// Resolve layers an app's overrides onto the platform defaults.
func Resolve(defaults Effective, s store.DeploySafetySettings) Effective {
	e := defaults
	if s.SuccessThreshold != nil {
		e.SuccessThreshold = *s.SuccessThreshold
	}
	if s.StableWindow != nil {
		e.StableWindow = *s.StableWindow
	}
	if s.DrainTimeout != nil {
		e.DrainTimeout = *s.DrainTimeout
	}
	if s.WatchWindow != nil {
		e.WatchWindow = *s.WatchWindow
	}
	if s.WatchMaxRestarts != nil {
		e.WatchMaxRestarts = *s.WatchMaxRestarts
	}
	if s.WatchErrorRate != nil {
		e.WatchErrorRate = *s.WatchErrorRate
	}
	if s.WatchMinRequests != nil {
		e.WatchMinRequests = *s.WatchMinRequests
	}
	switch {
	case s.WatchMode != nil:
		e.WatchMode = *s.WatchMode
	case s.AutoRollback != nil && *s.AutoRollback:
		e.WatchMode = WatchModeAuto
	case s.AutoRollback != nil && e.WatchMode == WatchModeAuto:
		e.WatchMode = WatchModeNotify
	}
	e.AutoRollback = e.WatchMode == WatchModeAuto
	if s.WatchLatencyRatio != nil {
		e.WatchLatencyRatio = *s.WatchLatencyRatio
	}
	if s.WatchSoftRollback != nil {
		e.WatchSoftRollback = *s.WatchSoftRollback
	}
	if e.SuccessThreshold < 1 {
		e.SuccessThreshold = 1
	}
	return e
}

// MaxOverrideWindow caps per-app durations so one app cannot stall the
// sequential reconcile loop or hold a watch open indefinitely.
const MaxOverrideWindow = 30 * time.Minute

// Validate rejects overrides outside the ranges the database enforces.
func Validate(s store.DeploySafetySettings) error {
	var errs []error
	if s.SuccessThreshold != nil && (*s.SuccessThreshold < 1 || *s.SuccessThreshold > 50) {
		errs = append(errs, errors.New("success_threshold must be between 1 and 50"))
	}
	windows := []struct {
		name string
		d    *time.Duration
	}{{"stable_window", s.StableWindow}, {"drain_timeout", s.DrainTimeout}, {"watch_window", s.WatchWindow}}
	for _, w := range windows {
		if w.d != nil && (*w.d < 0 || *w.d > MaxOverrideWindow) {
			errs = append(errs, fmt.Errorf("%s must be between 0s and %s", w.name, MaxOverrideWindow))
		}
	}
	if s.WatchMaxRestarts != nil && *s.WatchMaxRestarts < 1 {
		errs = append(errs, errors.New("watch_max_restarts must be at least 1"))
	}
	if s.WatchErrorRate != nil && (*s.WatchErrorRate <= 0 || *s.WatchErrorRate > 1) {
		errs = append(errs, errors.New("watch_error_rate must be a fraction in (0, 1]"))
	}
	if s.WatchMinRequests != nil && *s.WatchMinRequests < 1 {
		errs = append(errs, errors.New("watch_min_requests must be at least 1"))
	}
	if s.WatchMode != nil && !ValidWatchMode(*s.WatchMode) {
		errs = append(errs, errors.New("watch_mode must be off, notify or auto"))
	}
	if s.WatchLatencyRatio != nil && *s.WatchLatencyRatio != 0 && *s.WatchLatencyRatio < 1 {
		errs = append(errs, errors.New("watch_latency_ratio must be 0 (off) or at least 1"))
	}
	return errors.Join(errs...)
}

// ValidWatchMode reports whether m is off, notify or auto.
func ValidWatchMode(m string) bool {
	return m == WatchModeOff || m == WatchModeNotify || m == WatchModeAuto
}
