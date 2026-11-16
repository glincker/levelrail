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

// AdmitImagePull holds an image-pull deploy while the host is critical, reporting the reason once through note.
// It is a no-op without a gate or when pressure is fine, so default behaviour is unchanged.
func AdmitImagePull(ctx context.Context, note func(msg string)) error {
	g := hostGate.Load()
	if g == nil {
		return nil
	}
	return (*g).Admit(ctx, note)
}

// DerivedMaxConcurrent is the build slot count used when none is configured: one slot per two CPUs and per 4 GiB of RAM, at least 1.
func DerivedMaxConcurrent(cpus, memMB int) int {
	const mibPerBuild = 4096
	n := min(cpus/2, memMB/mibPerBuild)
	return max(n, 1)
}

var localPressure atomic.Pointer[func() string]

// SetLocalPressure wires the reader behind the control plane's own pressure level; nil removes it.
func SetLocalPressure(f func() string) {
	if f == nil {
		localPressure.Store(nil)
		return
	}
	localPressure.Store(&f)
}

// LocalPressure is the control plane host's pressure level, PressureUnknown before the guard runs.
func LocalPressure() string {
	if f := localPressure.Load(); f != nil {
		if v := (*f)(); v != "" {
			return v
		}
	}
	return PressureUnknown
}

// Process-wide routing policy, set from live settings at boot so a change applies to the next build.
var runnerPolicy atomic.Pointer[func(ctx context.Context) Policy]

// SetRunnerPolicyResolver wires the live routing settings; nil restores the original first-online choice.
func SetRunnerPolicyResolver(f func(ctx context.Context) Policy) {
	if f == nil {
		runnerPolicy.Store(nil)
		return
	}
	runnerPolicy.Store(&f)
}

func currentRunnerPolicy() func(ctx context.Context) Policy {
	if f := runnerPolicy.Load(); f != nil {
		return *f
	}
	return nil
}
