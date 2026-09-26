// Package preflight runs deterministic pre-deploy checks (DNS, ports, disk,
// memory, image and git reachability, env, mounts, GPU) and reports pass,
// warn or fail with a reason and fix hint for each. It only reads: nothing
// here writes a resource or influences the reconciler.
package preflight

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"
)

// Status is one check's outcome.
type Status string

// Check outcomes. Fail blocks a healthy deploy, warn is worth a look.
const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Check is one preflight result.
type Check struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status Status `json:"status"`
	Reason string `json:"reason"`
	Fix    string `json:"fix,omitempty"`
}

// Report is every check's result plus the worst status among them.
type Report struct {
	Status Status  `json:"status"`
	Checks []Check `json:"checks"`
}

// Request describes the app being checked. Zero fields skip the checks
// that depend on them.
type Request struct {
	Name        string
	NodeID      string
	Image       string
	Port        int
	HostPort    int
	Domains     []string
	MemoryBytes int64
	RequiredEnv []string
	EnvKeys     []string
	BindMounts  []string
	VolumePaths []string
	GPU         bool
	GitURL      string
	GitBranch   string
}

// Env holds the probes a Run uses. A nil probe skips its check, so a
// deployment without (say) GPU support simply omits it.
type Env struct {
	PublicIP       func(ctx context.Context) (string, error)
	LookupHost     func(ctx context.Context, host string) ([]string, error)
	HostPortHolder func(ctx context.Context, nodeID string, port int) (holder string, inUse bool, err error)
	FreeDisk       func(ctx context.Context, nodeID string) (int64, error)
	Memory         func(ctx context.Context, nodeID string) (total, available int64, err error)
	Image          ImageInspector
	Git            GitChecker
	GPUFit         func(ctx context.Context, nodeID string) error
	Limits         Limits
}

// Limits are the tunable thresholds, all overridable by environment.
type Limits struct {
	Timeout        time.Duration
	MinMemoryBytes int64
	DiskFactor     float64
	MinFreeDisk    int64
	ImageCacheTTL  time.Duration
}

const (
	defaultTimeout       = 8 * time.Second
	defaultMinMemory     = 128 << 20
	defaultDiskFactor    = 3.0
	defaultMinFreeDisk   = 2 << 30
	defaultImageCacheTTL = 5 * time.Minute
	envTimeout           = "APP_PREFLIGHT_TIMEOUT"
	envMinMemory         = "APP_PREFLIGHT_MIN_MEMORY_BYTES"
	envDiskFactor        = "APP_PREFLIGHT_DISK_FACTOR"
	envMinFreeDisk       = "APP_PREFLIGHT_MIN_FREE_DISK_BYTES"
	envImageCacheTTL     = "APP_PREFLIGHT_IMAGE_CACHE_TTL"
)

// LimitsFromEnv reads the thresholds, falling back to defaults.
func LimitsFromEnv() Limits {
	return Limits{
		Timeout:        envDuration(envTimeout, defaultTimeout),
		MinMemoryBytes: envInt(envMinMemory, defaultMinMemory),
		DiskFactor:     envFloat(envDiskFactor, defaultDiskFactor),
		MinFreeDisk:    envInt(envMinFreeDisk, defaultMinFreeDisk),
		ImageCacheTTL:  envDuration(envImageCacheTTL, defaultImageCacheTTL),
	}
}

func (l Limits) withDefaults() Limits {
	if l.Timeout <= 0 {
		l.Timeout = defaultTimeout
	}
	if l.MinMemoryBytes <= 0 {
		l.MinMemoryBytes = defaultMinMemory
	}
	if l.DiskFactor <= 0 {
		l.DiskFactor = defaultDiskFactor
	}
	if l.MinFreeDisk <= 0 {
		l.MinFreeDisk = defaultMinFreeDisk
	}
	return l
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

func envInt(key string, def int64) int64 {
	if n, err := strconv.ParseInt(os.Getenv(key), 10, 64); err == nil && n > 0 {
		return n
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if f, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && f > 0 {
		return f
	}
	return def
}

type checkFunc func(ctx context.Context) []Check

// Run executes every applicable check in parallel, each bounded by
// env.Limits.Timeout, and returns results in a stable order.
func Run(ctx context.Context, req Request, env Env) Report {
	lim := env.Limits.withDefaults()
	env.Limits = lim
	fns := []checkFunc{
		func(c context.Context) []Check { return checkDomains(c, req, env) },
		func(c context.Context) []Check { return checkHostPort(c, req, env) },
		func(c context.Context) []Check { return checkImageAndDisk(c, req, env) },
		func(c context.Context) []Check { return checkMemory(c, req, env) },
		func(c context.Context) []Check { return checkGit(c, req, env) },
		func(_ context.Context) []Check { return checkEnv(req) },
		func(_ context.Context) []Check { return checkMounts(req) },
		func(c context.Context) []Check { return checkGPU(c, req, env) },
	}
	results := make([][]Check, len(fns))
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, lim.Timeout)
			defer cancel()
			results[i] = fn(cctx)
		}()
	}
	wg.Wait()

	rep := Report{Status: StatusPass, Checks: []Check{}}
	for _, rs := range results {
		for _, c := range rs {
			rep.Checks = append(rep.Checks, c)
			rep.Status = worse(rep.Status, c.Status)
		}
	}
	return rep
}

func worse(a, b Status) Status {
	rank := map[Status]int{StatusPass: 0, StatusWarn: 1, StatusFail: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
