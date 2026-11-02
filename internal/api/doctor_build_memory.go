package api

import (
	"fmt"
	"os"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// doctorCheckBuildMemory warns, never fails, when the host is under the Next.js build floor.
func (rt *Router) doctorCheckBuildMemory() doctorCheckResource {
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
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d MiB of memory, builds capped at %d MiB when the setting is auto", hostMB, build.AutoMemoryCapMB(hostMB, build.MemGuardFromEnv(os.LookupEnv)))}
}
