package footprint

import (
	"sync"
	"time"
)

type cpuSample struct {
	at  time.Time
	cpu float64
}

var (
	winMu      sync.Mutex
	winSamples = []cpuSample{{at: startedAt}}
)

// WindowCPU is the average CPU percent over the last window, or since process start until a full window exists. Each call adds a sample, so a polling caller keeps the window populated.
func WindowCPU(window time.Duration, now time.Time, cpuSeconds float64) (percent, covered float64) {
	winMu.Lock()
	defer winMu.Unlock()
	winSamples = append(winSamples, cpuSample{at: now, cpu: cpuSeconds})
	cut := 0
	for i, s := range winSamples {
		if now.Sub(s.at) >= window {
			cut = i
		}
	}
	winSamples = winSamples[cut:]
	oldest := winSamples[0]
	dt := now.Sub(oldest.at).Seconds()
	if dt < 1 {
		return 0, dt
	}
	return (cpuSeconds - oldest.cpu) / dt * 100, dt
}

// Snapshot reads the process and its window CPU in one call.
func Snapshot(window time.Duration, apps int) Reading {
	p := ReadProc()
	pct, covered := WindowCPU(window, time.Now(), p.CPUSeconds)
	return Reading{
		RSSBytes: p.RSSBytes, CPUPercent: pct, CPUWindowSeconds: covered,
		Goroutines: p.Goroutines, OpenFDs: p.OpenFDs, Threads: p.Threads, Apps: apps,
	}
}
