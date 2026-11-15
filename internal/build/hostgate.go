package build

import (
	"context"
	"sync/atomic"
)

// HostGate lets the host pressure guard slow and hold new local builds.
type HostGate interface {
	Admit(ctx context.Context, wait func(msg string)) error
	BuildCap(limit int) int
}

var hostGate atomic.Pointer[HostGate]

// SetHostGate wires the guard at boot; nil removes it.
func SetHostGate(g HostGate) {
	if g == nil {
		hostGate.Store(nil)
		return
	}
	hostGate.Store(&g)
}

// admitLocal waits out critical host pressure for a build on this host and returns the concurrency cap to use.
// Builds on remote nodes are never held because the pressure reading is for this host only.
func admitLocal(ctx context.Context, node string, limit int, progress func(ProgressEvent)) (int, error) {
	g := hostGate.Load()
	if g == nil || node != "" {
		return limit, nil
	}
	err := (*g).Admit(ctx, func(msg string) {
		if progress != nil {
			progress(ProgressEvent{Step: "queue", Log: msg, Stream: "stderr"})
		}
	})
	if err != nil {
		return limit, err
	}
	return (*g).BuildCap(limit), nil
}

// DerivedMaxConcurrent is the build slot count used when none is configured: one slot per two CPUs and per 4 GiB of RAM, at least 1.
func DerivedMaxConcurrent(cpus, memMB int) int {
	const mibPerBuild = 4096
	n := min(cpus/2, memMB/mibPerBuild)
	return max(n, 1)
}
