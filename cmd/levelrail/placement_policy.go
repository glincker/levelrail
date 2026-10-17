package main

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/placement"
)

// placementPolicyFromEnv reads the overcommit and headroom settings. An
// invalid value keeps the safe default and logs a warning.
func placementPolicyFromEnv(logger *slog.Logger) placement.Policy {
	p := placement.DefaultPolicy()
	intEnv := func(name string, dst *int) {
		raw := os.Getenv(name)
		if raw == "" {
			return
		}
		v, err := strconv.Atoi(raw)
		if err != nil {
			logger.Warn("invalid "+name+", using the default", slog.String("value", raw))
			return
		}
		*dst = v
	}
	intEnv("APP_PLACEMENT_CPU_OVERCOMMIT_PERCENT", &p.CPUOvercommitPercent)
	intEnv("APP_PLACEMENT_MEMORY_OVERCOMMIT_PERCENT", &p.MemoryOvercommitPercent)
	intEnv("APP_PLACEMENT_MIN_FREE_DISK_PERCENT", &p.MinFreeDiskPercent)
	if raw := os.Getenv("APP_PLACEMENT_REJECT_UNKNOWN_CAPACITY"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			logger.Warn("invalid APP_PLACEMENT_REJECT_UNKNOWN_CAPACITY, using the default", slog.String("value", raw))
		} else {
			p.RejectUnknown = v
		}
	}
	if err := p.Validate(); err != nil {
		logger.Warn("invalid placement policy, using the defaults", slog.String("error", err.Error()))
		return placement.DefaultPolicy()
	}
	return p
}
