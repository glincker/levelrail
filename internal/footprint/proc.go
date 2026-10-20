package footprint

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Proc is a point in time reading of the process footprint.
type Proc struct {
	RSSBytes   uint64  `json:"rss_bytes"`
	HeapBytes  uint64  `json:"heap_bytes"`
	Goroutines int     `json:"goroutines"`
	Threads    int     `json:"threads"`
	OpenFDs    int     `json:"open_fds"`
	CPUSeconds float64 `json:"cpu_seconds"`
	CPUPercent float64 `json:"cpu_percent"`
}

var (
	cpuMu     sync.Mutex
	cpuPrevAt time.Time
	cpuPrev   float64
	startedAt = time.Now()
)

// ReadProc samples the process. CPUPercent is relative to the previous call (or process start for the first).
func ReadProc() Proc {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	p := Proc{HeapBytes: ms.HeapAlloc, Goroutines: runtime.NumGoroutine(), CPUSeconds: cpuSeconds()}
	p.RSSBytes, p.Threads = procStatus()
	p.OpenFDs = openFDs()
	if p.RSSBytes == 0 {
		p.RSSBytes = ms.Sys
	}
	now := time.Now()
	cpuMu.Lock()
	defer cpuMu.Unlock()
	prevAt, prevSec := cpuPrevAt, cpuPrev
	if prevAt.IsZero() {
		prevAt, prevSec = startedAt, 0
	}
	if dt := now.Sub(prevAt).Seconds(); dt >= 1 {
		p.CPUPercent = (p.CPUSeconds - prevSec) / dt * 100
		cpuPrevAt, cpuPrev = now, p.CPUSeconds
	}
	return p
}

// openFDs counts this process's file descriptors, or 0 where neither /proc nor /dev/fd exists.
func openFDs() int {
	for _, dir := range []string{"/proc/self/fd", "/dev/fd"} {
		if entries, err := os.ReadDir(dir); err == nil {
			return len(entries)
		}
	}
	return 0
}

func procStatus() (rss uint64, threads int) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "VmRSS:":
			if kb, err := strconv.ParseUint(f[1], 10, 64); err == nil {
				rss = kb << 10
			}
		case "Threads:":
			threads, _ = strconv.Atoi(f[1])
		}
	}
	return rss, threads
}
