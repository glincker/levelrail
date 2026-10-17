package models

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Compute kinds of a model resource.
const (
	ComputeGPU = store.ComputeGPU
	ComputeCPU = store.ComputeCPU
)

// ErrLicenseAck is returned when a non-curated repository is requested
// without the explicit licence acknowledgement.
var ErrLicenseAck = errors.New("models: non-curated repository needs a licence acknowledgement")

// CPUPolicy holds every CPU-mode guardrail. All values are environment
// driven so no threshold is hardcoded.
type CPUPolicy struct {
	// DefaultCPUs is used when a request sets no cpu cap; 0 means min(2, half the node).
	DefaultCPUs float64
	// DefaultMemoryPercent is the default memory cap as a share of node RAM.
	DefaultMemoryPercent float64
	// MaxCPUPercent and MaxMemoryPercent bound what a request may ask for.
	MaxCPUPercent    float64
	MaxMemoryPercent float64
	// FreeRAMFloorMiB is the RAM that must stay free for other workloads
	// after the memory cap is taken.
	FreeRAMFloorMiB int64
	CPUShares       int64
	OomScoreAdj     int
	PidsLimit       int64
	// DefaultContext is the context size when the request sets none.
	DefaultContext int
	// CachePercent and CacheMaxMiB bound llama.cpp's prompt cache (-cram).
	CachePercent float64
	CacheMaxMiB  int
	// MmapHeadroomMiB is the memory left beyond the weights below which
	// weights are loaded with --no-mmap; MmapMode forces "on" or "off".
	MmapHeadroomMiB int64
	MmapMode        string
	// OverheadMiB is the engine's fixed allowance in the fit estimate.
	OverheadMiB int64
	// ComfortPercent is the share of the cap a model may use to count as comfortable.
	ComfortPercent float64
	// BandwidthPerCPUGBs and BandwidthMaxGBs drive the tokens per second estimate.
	BandwidthPerCPUGBs float64
	BandwidthMaxGBs    float64
}

// LoadCPUPolicy reads the APP_MODEL_CPU_* variables.
func LoadCPUPolicy() CPUPolicy {
	return CPUPolicy{
		DefaultCPUs:          envFloat("APP_MODEL_CPU_DEFAULT_CPUS", 0),
		DefaultMemoryPercent: envFloat("APP_MODEL_CPU_DEFAULT_MEMORY_PERCENT", 25),
		MaxCPUPercent:        envFloat("APP_MODEL_CPU_MAX_CPU_PERCENT", 75),
		MaxMemoryPercent:     envFloat("APP_MODEL_CPU_MAX_MEMORY_PERCENT", 60),
		FreeRAMFloorMiB:      envInt64("APP_MODEL_CPU_FREE_RAM_FLOOR_MIB", 1024),
		CPUShares:            envInt64("APP_MODEL_CPU_SHARES", 128),
		OomScoreAdj:          int(envInt64("APP_MODEL_CPU_OOM_SCORE_ADJ", 800)),
		PidsLimit:            envInt64("APP_MODEL_CPU_PIDS_LIMIT", 256),
		DefaultContext:       int(envInt64("APP_MODEL_CPU_DEFAULT_CONTEXT", 4096)),
		CachePercent:         envFloat("APP_MODEL_CPU_CACHE_PERCENT", 5),
		CacheMaxMiB:          int(envInt64("APP_MODEL_CPU_CACHE_MAX_MIB", 512)),
		MmapHeadroomMiB:      envInt64("APP_MODEL_CPU_MMAP_HEADROOM_MIB", 1024),
		MmapMode:             strings.ToLower(strings.TrimSpace(os.Getenv("APP_MODEL_CPU_MMAP"))),
		OverheadMiB:          envInt64("APP_MODEL_CPU_OVERHEAD_MIB", 512),
		ComfortPercent:       envFloat("APP_MODEL_CPU_COMFORT_PERCENT", 85),
		BandwidthPerCPUGBs:   envFloat("APP_MODEL_CPU_BW_GBS_PER_CPU", 4),
		BandwidthMaxGBs:      envFloat("APP_MODEL_CPU_BW_GBS_MAX", 25),
	}
}

// HostFacts are the node figures CPU limits are derived from. Zero CPUs or
// MemTotalBytes means the node has not reported them.
type HostFacts struct {
	CPUs          int
	MemTotalBytes int64
	// MemAvailBytes is currently available memory, 0 when unknown.
	MemAvailBytes int64
}

// Known reports whether the node figures are usable.
func (h HostFacts) Known() bool { return h.CPUs > 0 && h.MemTotalBytes > 0 }

// CPULimits are the resolved hard caps of one CPU model.
type CPULimits struct {
	CPUMilli  int
	MemoryMiB int
	Context   int
}

// CPUBounds are the defaults and ceilings a deploy form offers for a node.
type CPUBounds struct {
	DefaultCPUMilli  int   `json:"default_cpu_milli"`
	DefaultMemoryMiB int   `json:"default_memory_mib"`
	MaxCPUMilli      int   `json:"max_cpu_milli"`
	MaxMemoryMiB     int   `json:"max_memory_mib"`
	FreeRAMFloorMiB  int64 `json:"free_ram_floor_mib"`
}

// Bounds reports the defaults and ceilings for h; zeros when h is unknown.
func (p CPUPolicy) Bounds(h HostFacts) CPUBounds {
	if !h.Known() {
		return CPUBounds{FreeRAMFloorMiB: p.FreeRAMFloorMiB}
	}
	def, _ := p.ResolveCPULimits(h, 0, 0, 0)
	totalMiB := int(h.MemTotalBytes / mib)
	maxMem := min(int(float64(totalMiB)*p.MaxMemoryPercent/100), max(totalMiB-int(p.FreeRAMFloorMiB), 0))
	return CPUBounds{DefaultCPUMilli: def.CPUMilli, DefaultMemoryMiB: def.MemoryMiB, FreeRAMFloorMiB: p.FreeRAMFloorMiB,
		MaxCPUMilli: max(int(float64(h.CPUs)*1000*p.MaxCPUPercent/100), 250), MaxMemoryMiB: maxMem}
}

// Threads is the llama.cpp thread count: the whole CPUs of the cap.
func (l CPULimits) Threads() int { return max(l.CPUMilli/1000, 1) }

// CacheMiB is the bounded prompt cache for the memory cap.
func (p CPUPolicy) CacheMiB(memoryMiB int) int {
	c := int(float64(memoryMiB) * p.CachePercent / 100)
	return max(min(c, p.CacheMaxMiB), 64)
}

// ResolveCPULimits turns a request (zeros mean default) into hard caps and
// refuses caps the node cannot honour.
func (p CPUPolicy) ResolveCPULimits(h HostFacts, reqCPUMilli, reqMemoryMiB, reqContext int) (CPULimits, error) {
	out := CPULimits{CPUMilli: reqCPUMilli, MemoryMiB: reqMemoryMiB, Context: reqContext}
	if out.Context <= 0 {
		out.Context = p.DefaultContext
	}
	if !h.Known() {
		if out.CPUMilli <= 0 || out.MemoryMiB <= 0 {
			return CPULimits{}, fmt.Errorf("%w: the node has not reported its CPUs and memory, so set both a cpu and a memory limit", ErrInvalid)
		}
		return out, nil
	}
	if out.CPUMilli <= 0 {
		def := p.DefaultCPUs * 1000
		if def <= 0 {
			def = math.Min(2000, float64(h.CPUs)*1000/2)
		}
		out.CPUMilli = max(int(def), 250)
	}
	totalMiB := int(h.MemTotalBytes / mib)
	if out.MemoryMiB <= 0 {
		out.MemoryMiB = int(float64(totalMiB) * p.DefaultMemoryPercent / 100)
	}
	if maxCPU := int(float64(h.CPUs) * 1000 * p.MaxCPUPercent / 100); out.CPUMilli > max(maxCPU, 250) {
		return CPULimits{}, fmt.Errorf("%w: %.2f CPUs is above the %.0f%% of this node's %d CPUs that a model may take (APP_MODEL_CPU_MAX_CPU_PERCENT)",
			ErrInvalid, float64(out.CPUMilli)/1000, p.MaxCPUPercent, h.CPUs)
	}
	if maxMem := int(float64(totalMiB) * p.MaxMemoryPercent / 100); out.MemoryMiB > maxMem {
		return CPULimits{}, fmt.Errorf("%w: %d MiB is above the %.0f%% of this node's %d MiB RAM that a model may take (APP_MODEL_CPU_MAX_MEMORY_PERCENT)",
			ErrInvalid, out.MemoryMiB, p.MaxMemoryPercent, totalMiB)
	}
	if left := int64(totalMiB - out.MemoryMiB); left < p.FreeRAMFloorMiB {
		return CPULimits{}, fmt.Errorf("%w: a %d MiB cap leaves %d MiB for other workloads, under the %d MiB floor (APP_MODEL_CPU_FREE_RAM_FLOOR_MIB)",
			ErrInvalid, out.MemoryMiB, left, p.FreeRAMFloorMiB)
	}
	return out, nil
}

// useNoMmap reads weights into memory when the cap leaves little room beyond
// them: mapped pages would thrash the disk, while a plain OOM kill is clear.
// Otherwise mmap lets the kernel drop weight pages under host pressure.
func (p CPUPolicy) useNoMmap(l CPULimits, weightsBytes int64) bool {
	switch p.MmapMode {
	case "off":
		return true
	case "on":
		return false
	}
	if weightsBytes <= 0 {
		return false
	}
	return int64(l.MemoryMiB)-weightsBytes/mib < p.MmapHeadroomMiB
}

// CPUCommand builds the llama.cpp server command line of a CPU model.
// localFile is the weights path inside the container, empty to let the
// engine pull the repository itself (remote nodes).
func CPUCommand(m Spec, l CPULimits, p CPUPolicy, localFile string, weightsBytes int64, embedding bool) []string {
	port := strconv.Itoa(engines[EngineLlamaCpp].Port)
	cmd := []string{}
	if localFile != "" {
		cmd = append(cmd, "-m", localFile)
	} else {
		cmd = append(cmd, "-hf", m.ModelRef)
	}
	t := strconv.Itoa(l.Threads())
	cmd = append(cmd, "--alias", m.ModelRef, "--host", "0.0.0.0", "--port", port, "--metrics",
		"-t", t, "-tb", t, "-np", "1", "-c", strconv.Itoa(l.Context), "-cram", strconv.Itoa(p.CacheMiB(l.MemoryMiB)), "--jinja")
	if p.useNoMmap(l, weightsBytes) {
		cmd = append(cmd, "--no-mmap")
	}
	if embedding {
		cmd = append(cmd, "--embedding")
	}
	return cmd
}
