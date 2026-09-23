package api

import (
	"fmt"
	"runtime"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// defaultDoctorMinRAMBytes/defaultDoctorMinCPUCount are the recommended
// minimums GET /api/v1/system/doctor's ram/cpu checks warn below when
// WithDoctorMinRAMBytes/WithDoctorMinCPUCount haven't overridden them.
// Section 1's own target audience (3-50 services on 1-10 machines) is
// comfortable above these; below them isn't a hard failure, since a
// small single-app instance can run lighter.
const (
	defaultDoctorMinRAMBytes = 1 << 30 // 1GiB
	defaultDoctorMinCPUCount = 2
)

func (rt *Router) doctorCheckRAM() doctorCheckResource {
	const code, name = "ram", "RAM"
	read := rt.doctorHostMemory
	if read == nil {
		read = func() (int64, int64, error) { return telemetry.HostMemoryBytes() }
	}
	total, _, err := read()
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read total memory: %s", err)}
	}

	threshold := rt.doctorMinRAMBytes
	if threshold <= 0 {
		threshold = defaultDoctorMinRAMBytes
	}
	if total < threshold {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d bytes total, below the %d byte recommended minimum", total, threshold),
			Fix:      "Add more RAM, or reduce the number of apps and replicas running on this box.",
			DocsPath: "/troubleshooting#below-the-recommended-minimum-ram-or-cpu",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d bytes total", total)}
}

func (rt *Router) doctorCheckCPU() doctorCheckResource {
	const code, name = "cpu", "CPU cores"
	count := rt.doctorNumCPU
	if count == nil {
		count = runtime.NumCPU
	}
	n := count()

	threshold := rt.doctorMinCPUCount
	if threshold <= 0 {
		threshold = defaultDoctorMinCPUCount
	}
	if n < threshold {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d core(s), below the %d core recommended minimum", n, threshold),
			Fix:      "Move to a host with more CPU cores, or run fewer concurrent builds and apps on this one.",
			DocsPath: "/troubleshooting#below-the-recommended-minimum-ram-or-cpu",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d core(s)", n)}
}
