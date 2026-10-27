package compose

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// deployLimits is deploy.resources.limits, the Compose spelling of a hard cap.
type deployLimits struct {
	CPUs   yamlScalarString `yaml:"cpus"`
	Memory yamlScalarString `yaml:"memory"`
}

var memoryUnits = map[string]int64{
	"": 1, "b": 1,
	"k": 1 << 10, "kb": 1 << 10,
	"m": 1 << 20, "mb": 1 << 20,
	"g": 1 << 30, "gb": 1 << 30,
}

// Env caps on compose limits; unset leaves only the int64 headroom guard,
// except shm, whose tmpfs is not bounded by mem_limit's absence.
const (
	envMaxMemoryBytes = "APP_COMPOSE_MAX_MEMORY_BYTES"
	envMaxCPUs        = "APP_COMPOSE_MAX_CPUS"
	envMaxShmBytes    = "APP_COMPOSE_MAX_SHM_BYTES"
	// Headroom so quota SUMs over many services cannot overflow int64.
	sumHeadroomShift = 16
	// defaultMaxShmBytes applies when APP_COMPOSE_MAX_SHM_BYTES is unset.
	defaultMaxShmBytes = 2 << 30
)

func envCap(name string, fallback float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(name)), 64); err == nil && v > 0 && !math.IsInf(v, 0) {
		return math.Min(v, fallback)
	}
	return fallback
}

// toInt64 converts a scaled float, refusing NaN, Inf, overflow and values that round to zero.
func toInt64(v, ceiling float64) (int64, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 1 || v > ceiling {
		return 0, false
	}
	return int64(v), true
}

// parseMemoryBytes reads Compose's byte-value form ("512m", "1.5g", "1048576").
func parseMemoryBytes(raw string) (int64, error) {
	return parseByteSize(raw, envMaxMemoryBytes)
}

func parseByteSize(raw, capEnv string) (int64, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	i := len(s)
	for i > 0 && (s[i-1] < '0' || s[i-1] > '9') && s[i-1] != '.' {
		i--
	}
	mult, ok := memoryUnits[s[i:]]
	n, err := strconv.ParseFloat(s[:i], 64)
	if !ok || err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0, fmt.Errorf("%q is not a memory size such as 512m or 1g", raw)
	}
	memCap := envCap(capEnv, float64(math.MaxInt64>>sumHeadroomShift))
	if capEnv == envMaxShmBytes && strings.TrimSpace(os.Getenv(capEnv)) == "" {
		memCap = defaultMaxShmBytes
	}
	b, ok := toInt64(n*float64(mult), memCap)
	if !ok {
		return 0, fmt.Errorf("%q is out of range: it must be at least 1 byte and at most %d bytes (%s)", raw, int64(memCap), capEnv)
	}
	return b, nil
}

func parseNanoCPUs(raw string) (int64, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0, fmt.Errorf("%q is not a positive CPU count such as 0.5 or 2", raw)
	}
	maxCPUs := envCap(envMaxCPUs, float64(math.MaxInt64>>sumHeadroomShift)/1e9)
	nano, ok := toInt64(n*1e9, maxCPUs*1e9)
	if !ok {
		return 0, fmt.Errorf("%q is out of range: it must be at least 0.000000001 and at most %g CPUs (%s)", raw, maxCPUs, envMaxCPUs)
	}
	return nano, nil
}

// limitsForService merges deploy.resources.limits with the service-level
// mem_limit and cpus shorthands (the shorthand wins, as in Compose v2).
func limitsForService(svc Service) (mem, nano int64, err error) {
	memRaw, cpuRaw := string(svc.MemLimit), string(svc.CPUs)
	if svc.Deploy != nil {
		l := svc.Deploy.Resources.Limits
		if memRaw == "" {
			memRaw = string(l.Memory)
		}
		if cpuRaw == "" {
			cpuRaw = string(l.CPUs)
		}
	}
	if memRaw != "" {
		if mem, err = parseMemoryBytes(memRaw); err != nil {
			return 0, 0, fmt.Errorf("mem_limit: %w", err)
		}
	}
	if cpuRaw != "" {
		if nano, err = parseNanoCPUs(cpuRaw); err != nil {
			return 0, 0, fmt.Errorf("cpus: %w", err)
		}
	}
	return mem, nano, nil
}

func applyLimits(d *store.DesiredService, mem, nano int64) {
	if mem == 0 && nano == 0 {
		return
	}
	if d.Resources == nil {
		d.Resources = &store.ServiceResources{}
	}
	d.Resources.MemoryBytes, d.Resources.NanoCPUs = mem, nano
}

// applyShmSize sets /dev/shm from shm_size:, with mem_limit's unit and range rules.
func applyShmSize(d *store.DesiredService, raw yamlScalarString) error {
	if raw == "" {
		return nil
	}
	b, err := parseByteSize(string(raw), envMaxShmBytes)
	if err != nil {
		return fmt.Errorf("shm_size: %w", err)
	}
	if d.Resources == nil {
		d.Resources = &store.ServiceResources{}
	}
	d.Resources.ShmSizeBytes = b
	return nil
}
