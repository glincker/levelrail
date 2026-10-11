package resourceadvisor

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	mib = 1024 * 1024
	gib = 1024 * mib
	day = 24 * time.Hour
)

// Thresholds tune every rule. Each field has an APP_RESOURCE_ADVISOR_* env var.
type Thresholds struct {
	Window             time.Duration
	MinData            time.Duration
	MemoryHeadroom     float64
	CPUHeadroom        float64
	TightRatio         float64
	OverProvisionRatio float64
	RestartsTight      int
	GrowthMinFraction  float64
	GrowthMinFit       float64
	IdleWindow         time.Duration
	IdleCPUPercent     float64
	SleepIdleMinutes   int64
	TuningMinMemory    int64
	SmallDatabaseBytes int64
	SmallDatabaseCount int
	HostPressure       float64
	SwapWarnBytes      int64
	TopConsumerShare   float64
	DiskHorizon        time.Duration
	ReclaimableMin     int64
	CacheTTL           time.Duration
}

// DefaultThresholds are the documented defaults (docs/resource-advisor.md).
func DefaultThresholds() Thresholds {
	return Thresholds{
		Window:             7 * day,
		MinData:            day,
		MemoryHeadroom:     1.3,
		CPUHeadroom:        1.5,
		TightRatio:         0.9,
		OverProvisionRatio: 0.4,
		RestartsTight:      3,
		GrowthMinFraction:  0.2,
		GrowthMinFit:       0.8,
		IdleWindow:         7 * day,
		IdleCPUPercent:     1,
		SleepIdleMinutes:   30,
		TuningMinMemory:    192 * mib,
		SmallDatabaseBytes: 256 * mib,
		SmallDatabaseCount: 4,
		HostPressure:       0.9,
		SwapWarnBytes:      256 * mib,
		TopConsumerShare:   0.6,
		DiskHorizon:        30 * day,
		ReclaimableMin:     gib,
		CacheTTL:           time.Minute,
	}
}

// EnvPrefix starts every threshold env var.
const EnvPrefix = "APP_RESOURCE_ADVISOR_"

// ThresholdsFromEnv overlays env values on the defaults. A bad value keeps
// its default and is reported, so a typo never disables the advisor.
func ThresholdsFromEnv(lookup func(string) (string, bool)) (Thresholds, []error) {
	t := DefaultThresholds()
	var errs []error
	dur := func(key string, dst *time.Duration) {
		if v, ok := lookup(EnvPrefix + key); ok && strings.TrimSpace(v) != "" {
			d, err := parseDuration(v)
			if err != nil || d <= 0 {
				errs = append(errs, fmt.Errorf("%s%s=%q: want a positive duration such as 24h or 7d", EnvPrefix, key, v))
				return
			}
			*dst = d
		}
	}
	ratio := func(key string, dst *float64, lo, hi float64) {
		if v, ok := lookup(EnvPrefix + key); ok && strings.TrimSpace(v) != "" {
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil || f <= lo || f > hi {
				errs = append(errs, fmt.Errorf("%s%s=%q: want a number above %g and at most %g", EnvPrefix, key, v, lo, hi))
				return
			}
			*dst = f
		}
	}
	size := func(key string, dst *int64) {
		if v, ok := lookup(EnvPrefix + key); ok && strings.TrimSpace(v) != "" {
			n, err := parseBytes(v)
			if err != nil || n <= 0 {
				errs = append(errs, fmt.Errorf("%s%s=%q: want a size such as 256Mi or 1Gi", EnvPrefix, key, v))
				return
			}
			*dst = n
		}
	}
	count := func(key string, dst *int64, lo int64) {
		if v, ok := lookup(EnvPrefix + key); ok && strings.TrimSpace(v) != "" {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil || n < lo {
				errs = append(errs, fmt.Errorf("%s%s=%q: want a whole number of at least %d", EnvPrefix, key, v, lo))
				return
			}
			*dst = n
		}
	}
	dur("WINDOW", &t.Window)
	dur("MIN_DATA", &t.MinData)
	ratio("MEMORY_HEADROOM", &t.MemoryHeadroom, 1, 4)
	ratio("CPU_HEADROOM", &t.CPUHeadroom, 1, 4)
	ratio("TIGHT_RATIO", &t.TightRatio, 0, 1)
	ratio("OVERPROVISION_RATIO", &t.OverProvisionRatio, 0, 1)
	restarts := int64(t.RestartsTight)
	count("RESTARTS_TIGHT", &restarts, 1)
	t.RestartsTight = int(restarts)
	ratio("GROWTH_MIN", &t.GrowthMinFraction, 0, 10)
	ratio("GROWTH_FIT", &t.GrowthMinFit, 0, 1)
	dur("IDLE_WINDOW", &t.IdleWindow)
	ratio("IDLE_CPU_PERCENT", &t.IdleCPUPercent, 0, 100)
	count("SLEEP_MINUTES", &t.SleepIdleMinutes, 5)
	size("TUNING_MIN_MEMORY", &t.TuningMinMemory)
	size("SMALL_DB_MEMORY", &t.SmallDatabaseBytes)
	small := int64(t.SmallDatabaseCount)
	count("SMALL_DB_COUNT", &small, 2)
	t.SmallDatabaseCount = int(small)
	ratio("HOST_PRESSURE", &t.HostPressure, 0, 1)
	size("SWAP_WARN", &t.SwapWarnBytes)
	ratio("TOP_SHARE", &t.TopConsumerShare, 0, 1)
	dur("DISK_HORIZON", &t.DiskHorizon)
	size("RECLAIM_MIN", &t.ReclaimableMin)
	dur("CACHE_TTL", &t.CacheTTL)
	if t.OverProvisionRatio >= t.TightRatio {
		errs = append(errs, fmt.Errorf("%sOVERPROVISION_RATIO must be below %sTIGHT_RATIO, using defaults", EnvPrefix, EnvPrefix))
		d := DefaultThresholds()
		t.OverProvisionRatio, t.TightRatio = d.OverProvisionRatio, d.TightRatio
	}
	return t, errs
}

// parseDuration accepts Go durations plus a whole-day "7d" form.
func parseDuration(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if n, ok := strings.CutSuffix(v, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("parse days %q: %w", v, err)
		}
		return time.Duration(days) * day, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", v, err)
	}
	return d, nil
}

// parseBytes accepts plain bytes or Ki, Mi, Gi suffixes.
func parseBytes(v string) (int64, error) {
	v = strings.TrimSpace(v)
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"Ki", 1024}, {"Mi", mib}, {"Gi", gib}} {
		if n, ok := strings.CutSuffix(v, u.suffix); ok {
			v, mult = n, u.mult
			break
		}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse size %q: %w", v, err)
	}
	return n * mult, nil
}
