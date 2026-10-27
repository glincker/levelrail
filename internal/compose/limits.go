package compose

import (
	"fmt"
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

// parseMemoryBytes reads Compose's byte-value form ("512m", "1.5g", "1048576").
func parseMemoryBytes(raw string) (int64, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	i := len(s)
	for i > 0 && (s[i-1] < '0' || s[i-1] > '9') && s[i-1] != '.' {
		i--
	}
	mult, ok := memoryUnits[s[i:]]
	n, err := strconv.ParseFloat(s[:i], 64)
	if !ok || err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a memory size such as 512m or 1g", raw)
	}
	return int64(n * float64(mult)), nil
}

func parseNanoCPUs(raw string) (int64, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a positive CPU count such as 0.5 or 2", raw)
	}
	return int64(n * 1e9), nil
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
			return 0, 0, fmt.Errorf("memory limit: %w", err)
		}
	}
	if cpuRaw != "" {
		if nano, err = parseNanoCPUs(cpuRaw); err != nil {
			return 0, 0, fmt.Errorf("cpus limit: %w", err)
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
