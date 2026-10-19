// Package footprint bounds and measures the control plane's idle cost: a default memory limit, a registry of background loops, and process stats.
package footprint

import (
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// Env vars read by ApplyMemoryLimit.
const (
	EnvMemLimit        = "GOMEMLIMIT"
	EnvMemLimitPercent = "APP_GOMEMLIMIT_PERCENT"
	DefaultPercent     = 80
	maxPercent         = 95
	minLimitBytes      = 128 << 20
	cgroupV2Path       = "/sys/fs/cgroup/memory.max"
	cgroupV1Path       = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
	meminfoPath        = "/proc/meminfo"
	unlimitedCgroup    = int64(1) << 60
)

// MemoryLimit describes what ApplyMemoryLimit decided.
type MemoryLimit struct {
	LimitBytes int64  `json:"limit_bytes"`
	Source     string `json:"source"`
	Percent    int    `json:"percent"`
}

// Percent reads APP_GOMEMLIMIT_PERCENT; 0 disables the default limit.
func Percent(getenv func(string) string) int {
	v := strings.TrimSpace(getenv(EnvMemLimitPercent))
	if v == "" {
		return DefaultPercent
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > maxPercent {
		return DefaultPercent
	}
	return n
}

// LimitFor returns percent of total bytes, or 0 when the result would be too small to be safe.
func LimitFor(total int64, percent int) int64 {
	if total <= 0 || percent <= 0 {
		return 0
	}
	l := total / 100 * int64(percent)
	if l < minLimitBytes {
		return 0
	}
	return l
}

// ApplyMemoryLimit sets a soft Go memory limit from the container or host memory unless GOMEMLIMIT is already set.
func ApplyMemoryLimit(getenv func(string) string, logger *slog.Logger) MemoryLimit {
	if getenv == nil {
		getenv = os.Getenv
	}
	pct := Percent(getenv)
	if getenv(EnvMemLimit) != "" {
		applied = MemoryLimit{LimitBytes: debug.SetMemoryLimit(-1), Source: "env", Percent: pct}
		return applied
	}
	total, source := systemMemory()
	limit := LimitFor(total, pct)
	if limit == 0 {
		applied = MemoryLimit{Source: "none", Percent: pct}
		return applied
	}
	debug.SetMemoryLimit(limit)
	if logger != nil {
		logger.Info("footprint: memory limit set", slog.Int64("limit_bytes", limit), slog.String("source", source), slog.Int("percent", pct))
	}
	applied = MemoryLimit{LimitBytes: limit, Source: source, Percent: pct}
	return applied
}

var applied MemoryLimit

// CurrentMemoryLimit returns what the last ApplyMemoryLimit call decided.
func CurrentMemoryLimit() MemoryLimit { return applied }

func systemMemory() (int64, string) {
	for _, path := range []string{cgroupV2Path, cgroupV1Path} {
		if n, ok := readInt(path); ok && n < unlimitedCgroup {
			return n, "cgroup"
		}
	}
	if b, err := os.ReadFile(meminfoPath); err == nil {
		if n := parseMemTotal(string(b)); n > 0 {
			return n, "host"
		}
	}
	return 0, "none"
}

func readInt(path string) (int64, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // callers pass fixed cgroup paths
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return n, err == nil
}

func parseMemTotal(meminfo string) int64 {
	for _, line := range strings.Split(meminfo, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "MemTotal:" {
			kb, err := strconv.ParseInt(f[1], 10, 64)
			if err != nil {
				return 0
			}
			return kb << 10
		}
	}
	return 0
}
