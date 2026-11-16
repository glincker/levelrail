package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Env seeds and defaults for the build memory guard. The cap itself is the
// builds.memory_limit_mb runtime setting (APP_BUILD_MEMORY_MB seeds it).
const (
	EnvBuildMemoryPercent  = "APP_BUILD_MEMORY_PERCENT"
	EnvBuildMemoryReserve  = "APP_BUILD_MEMORY_RESERVE_MB"
	EnvBuildNodeHeapPct    = "APP_BUILD_NODE_HEAP_PERCENT"
	DefaultMemoryPercent   = 60
	DefaultMemoryReserveMB = 512
	DefaultNodeHeapPercent = 75

	// BuildMemoryFloorMB is the smallest auto cap worth setting; BuildMemoryMinMB is the registry minimum.
	BuildMemoryFloorMB = 512
	BuildMemoryMinMB   = 256
)

// MemGuard holds the knobs that turn host memory into a build cap.
type MemGuard struct {
	Percent         int
	ReserveMB       int
	NodeHeapPercent int
}

// MemGuardFromEnv reads the seeds; a malformed or out of range value falls back to the default.
func MemGuardFromEnv(lookup func(string) (string, bool)) MemGuard {
	g := MemGuard{Percent: DefaultMemoryPercent, ReserveMB: DefaultMemoryReserveMB, NodeHeapPercent: DefaultNodeHeapPercent}
	read := func(name string, lo, hi int, dst *int) {
		if v, ok := lookup(name); ok && v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= lo && n <= hi {
				*dst = n
			}
		}
	}
	read(EnvBuildMemoryPercent, 1, 100, &g.Percent)
	read(EnvBuildMemoryReserve, 0, 1<<20, &g.ReserveMB)
	read(EnvBuildNodeHeapPct, 10, 100, &g.NodeHeapPercent)
	return g
}

// AutoMemoryCapMB is min(max(host*percent, floor), host-reserve), never below the registry minimum.
// Unknown host memory (0) yields 0, meaning no cap.
func AutoMemoryCapMB(hostMB int, g MemGuard) int {
	if hostMB <= 0 {
		return 0
	}
	c := hostMB * g.Percent / 100
	if c < BuildMemoryFloorMB {
		c = BuildMemoryFloorMB
	}
	if room := hostMB - g.ReserveMB; c > room {
		c = room
	}
	if c < BuildMemoryMinMB {
		c = BuildMemoryMinMB
	}
	return c
}

// EffectiveMemoryMB turns the setting into a cap: a positive value is used as is, 0 means auto.
func EffectiveMemoryMB(setting, hostMB int, g MemGuard) int {
	if setting > 0 {
		return setting
	}
	return AutoMemoryCapMB(hostMB, g)
}

// NodeHeapMB is the V8 old space size that leaves room for the rest of a Node build inside the cap.
func NodeHeapMB(capMB, percent int) int {
	if capMB <= 0 || percent <= 0 {
		return 0
	}
	return capMB * percent / 100
}

// Framework build memory floors in MiB of host RAM. Documented guidance, not measured:
// no build on a real host has been timed for these numbers yet.
const (
	FrameworkNextJS = "nextjs"
	FrameworkOther  = "other"
)

var frameworkMemoryFloorMB = map[string]int{
	FrameworkNextJS: 2048,
	FrameworkOther:  1024,
}

// FrameworkMemoryFloorMB returns the host memory below which a build of the framework is likely to run out of memory.
func FrameworkMemoryFloorMB(framework string) int {
	if n, ok := frameworkMemoryFloorMB[framework]; ok {
		return n
	}
	return frameworkMemoryFloorMB[FrameworkOther]
}

// DetectBuildFramework reports FrameworkNextJS when dir's package.json depends on next, else FrameworkOther.
func DetectBuildFramework(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "package.json")) //nolint:gosec // dir is the checkout the build already reads
	if err != nil {
		return FrameworkOther
	}
	var pkg struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return FrameworkOther
	}
	if _, ok := pkg.Dependencies["next"]; ok {
		return FrameworkNextJS
	}
	if _, ok := pkg.DevDependencies["next"]; ok {
		return FrameworkNextJS
	}
	return FrameworkOther
}

// LowMemoryWarning returns a one line warning when the host is under the framework's floor, else "".
func LowMemoryWarning(framework string, hostMB int) string {
	floor := FrameworkMemoryFloorMB(framework)
	if hostMB <= 0 || hostMB >= floor {
		return ""
	}
	return fmt.Sprintf("warning: this host has %d MiB of memory and a %s build is documented to want %d MiB (documented guidance, not measured). Use a builds-only node, add swap, or build elsewhere and deploy the image.", hostMB, framework, floor)
}

// MemoryKillMessage explains an out of memory build kill: what was set, what the host has, what to do.
func MemoryKillMessage(limitMB, hostMB int) string {
	return fmt.Sprintf("the kernel out of memory killer stopped the build (exit 137, OOMKilled). Setting builds.memory_limit_mb allowed %d MiB and the host has %d MiB. Options: raise builds.memory_limit_mb, run builds on a builds-only node, add swap to the host, or build elsewhere (for example in CI) and deploy the image", limitMB, hostMB)
}
