package api

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

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

// doctorDiskIOSampleBytes is how much data disk_io_latency writes and
// fsyncs to measure write latency: enough to move past OS write-cache
// buffering noise, small enough that the check itself never becomes the
// slow thing an operator is troubleshooting.
const doctorDiskIOSampleBytes = 1 << 20 // 1MiB

// defaultDoctorDiskIOWarnLatency is the write+fsync duration disk_io_latency
// warns above when WithDoctorDiskIOWarnLatency hasn't overridden it. A
// nearly-full disk can still pass disk_space on free bytes alone while its
// write latency has degraded to something that makes every deploy and log
// write feel stuck; this catches that case directly instead of only ever
// inferring it from unrelated symptoms.
const defaultDoctorDiskIOWarnLatency = 200 * time.Millisecond

func (rt *Router) doctorCheckDiskIOLatency() doctorCheckResource {
	const code, name = "disk_io_latency", "Disk write latency"
	if rt.dataDir == "" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "data directory not configured"}
	}

	f, err := os.CreateTemp(rt.dataDir, ".doctor-io-check-*")
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not create a test file: %s", err)}
	}
	path := f.Name()
	defer func() { _ = os.Remove(path) }()

	start := time.Now()
	writeErr := func() error {
		if _, err := f.Write(make([]byte, doctorDiskIOSampleBytes)); err != nil {
			return err
		}
		return f.Sync()
	}()
	elapsed := time.Since(start)
	_ = f.Close()
	if writeErr != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not measure write latency: %s", writeErr)}
	}

	threshold := rt.doctorDiskIOWarnLatency
	if threshold <= 0 {
		threshold = defaultDoctorDiskIOWarnLatency
	}
	if elapsed > threshold {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("writing and fsyncing 1MiB took %s, above the %s warning threshold; free space alone won't show this", elapsed.Round(time.Millisecond), threshold),
			Fix:      "Check for a saturated disk (iostat, iotop), a network volume under load, or a nearly-full disk whose remaining space is fragmented.",
			DocsPath: "/troubleshooting#disk-write-latency-is-high",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("wrote and fsynced 1MiB in %s", elapsed.Round(time.Millisecond))}
}

func (rt *Router) doctorCheckRAM() doctorCheckResource {
	const code, name = "ram", "RAM"
	read := rt.doctorHostMemory
	if read == nil {
		read = func() (int64, int64, error) { return telemetry.HostMemoryBytes() }
	}
	total, _, err := read()
	if errors.Is(err, telemetry.ErrHostMemoryUnsupported) {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Memory size is not reported on this operating system."}
	}
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
