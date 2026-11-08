package footprint

import (
	"sync"
	"time"
)

// maxBootStages bounds the record so a restart loop inside one process cannot grow it.
const maxBootStages = 64

// BootStage is how long one named startup step took.
type BootStage struct {
	Name       string  `json:"name"`
	DurationMS float64 `json:"duration_ms"`
}

var boot struct {
	mu     sync.Mutex
	stages []BootStage
	total  time.Duration
}

// RecordBootStage notes one finished startup step. Past the bound it only adds to the total.
func RecordBootStage(name string, d time.Duration) {
	boot.mu.Lock()
	defer boot.mu.Unlock()
	boot.total += d
	if len(boot.stages) < maxBootStages {
		boot.stages = append(boot.stages, BootStage{Name: name, DurationMS: float64(d) / float64(time.Millisecond)})
	}
}

// BootStages returns a copy of the recorded steps in start order.
func BootStages() []BootStage {
	boot.mu.Lock()
	defer boot.mu.Unlock()
	return append([]BootStage(nil), boot.stages...)
}

// BootTotal is the summed time of every recorded step.
func BootTotal() time.Duration {
	boot.mu.Lock()
	defer boot.mu.Unlock()
	return boot.total
}

// TimeBootStage runs fn and records its duration, returning fn's error unchanged.
func TimeBootStage(name string, fn func() error) error {
	start := time.Now()
	err := fn()
	RecordBootStage(name, time.Since(start))
	return err
}

func resetBootForTest() {
	boot.mu.Lock()
	defer boot.mu.Unlock()
	boot.stages, boot.total = nil, 0
}
