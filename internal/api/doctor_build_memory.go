package api

import (
	"context"
	"fmt"
	"os"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// doctorCheckBuildMemory warns, never fails, when the host is under the Next.js build floor.
func (rt *Router) doctorCheckBuildMemory(ctx context.Context) doctorCheckResource {
	const code, name = "build_memory", "Build memory"
	read := rt.doctorHostMemory
	if read == nil {
		read = func() (int64, int64, error) { return telemetry.HostMemoryBytes() }
	}
	total, _, err := read()
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Host memory could not be read, so build sizing was not checked."}
	}
	hostMB := int(total >> 20)
	floor := build.FrameworkMemoryFloorMB(build.FrameworkNextJS)
	if hostMB < floor {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d MiB of memory is under the %d MiB a Next.js build is documented to want (documented guidance, not measured); the build cap is limited to protect running apps", hostMB, floor),
			Fix:      "Use a builds-only node, add swap, or build in CI and deploy the image. Raise builds.memory_limit_mb only if the host can spare it.",
			DocsPath: "/build-performance#sizing-a-vps",
		}
	}
	guard := build.MemGuardFromEnv(os.LookupEnv)
	// Stored value wins over the env seed; the service resolves that order.
	set := build.LimitsFromEnv(os.LookupEnv).MemoryMB
	if rt.runtimeSettings != nil {
		set = rt.runtimeSettings.Int(ctx, runtimesettings.KeyBuildsMemoryLimitMB)
	}
	if set > hostMB {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message: fmt.Sprintf("builds.memory_limit_mb is %d MiB but the host has %d MiB of memory; a build can exhaust the host before the cap applies", set, hostMB),
			Fix:     fmt.Sprintf("Set builds.memory_limit_mb to 0 (auto, %d MiB here) or to a value under the host memory.", build.AutoMemoryCapMB(hostMB, guard)),
		}
	}
	if set > 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d MiB of memory, builds capped at %d MiB by the builds.memory_limit_mb setting", hostMB, set)}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d MiB of memory, builds capped at %d MiB when the setting is auto", hostMB, build.AutoMemoryCapMB(hostMB, guard))}
}
