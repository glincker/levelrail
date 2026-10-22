// Package benchreport turns benchmark runs into a results file that records the host, build and method next to the numbers.
package benchreport

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Docker is what the Docker daemon reports about itself.
type Docker struct {
	Version  string `json:"version,omitempty"`
	OS       string `json:"os,omitempty"`
	MemBytes int64  `json:"mem_bytes,omitempty"`
}

// Host describes the machine a run came from.
type Host struct {
	OS        string  `json:"os"`
	Arch      string  `json:"arch"`
	OSVersion string  `json:"os_version,omitempty"`
	Kernel    string  `json:"kernel,omitempty"`
	CPUModel  string  `json:"cpu_model,omitempty"`
	CPUs      int     `json:"cpus"`
	MemBytes  uint64  `json:"mem_bytes,omitempty"`
	Docker    Docker  `json:"docker"`
	LoadStart float64 `json:"load1_start"`
	LoadEnd   float64 `json:"load1_end"`
	Note      string  `json:"note,omitempty"`
}

// Probe is the set of system reads DetectHost uses, replaceable in tests.
type Probe struct {
	GOOS     string
	GOARCH   string
	NumCPU   int
	ReadFile func(path string) ([]byte, error)
	Run      func(ctx context.Context, name string, args ...string) (string, error)
}

// DefaultProbe reads the real machine.
func DefaultProbe() Probe {
	return Probe{
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, NumCPU: runtime.NumCPU(),
		ReadFile: os.ReadFile,
		Run: func(ctx context.Context, name string, args ...string) (string, error) {
			out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // DetectHost passes fixed names and arguments
			return strings.TrimSpace(string(out)), err
		},
	}
}

// DetectHost fills a Host from p; fields a platform cannot report stay empty rather than guessed.
func DetectHost(ctx context.Context, p Probe) Host {
	h := Host{OS: p.GOOS, Arch: p.GOARCH, CPUs: p.NumCPU}
	switch p.GOOS {
	case "darwin":
		h.CPUModel = runOut(ctx, p, "sysctl", "-n", "machdep.cpu.brand_string")
		if v := runOut(ctx, p, "sysctl", "-n", "hw.memsize"); v != "" {
			h.MemBytes, _ = strconv.ParseUint(v, 10, 64)
		}
		h.OSVersion = runOut(ctx, p, "sw_vers", "-productVersion")
		h.Kernel = runOut(ctx, p, "uname", "-r")
	case "linux":
		if p.ReadFile == nil {
			break
		}
		if b, err := p.ReadFile("/proc/cpuinfo"); err == nil {
			h.CPUModel = ParseCPUInfoModel(string(b))
		}
		if b, err := p.ReadFile("/proc/meminfo"); err == nil {
			h.MemBytes = ParseMemTotal(string(b))
		}
		if b, err := p.ReadFile("/etc/os-release"); err == nil {
			h.OSVersion = ParseOSRelease(string(b))
		}
		if b, err := p.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			h.Kernel = strings.TrimSpace(string(b))
		}
	}
	h.LoadStart = Load1(ctx, p)
	return h
}

func runOut(ctx context.Context, p Probe, name string, args ...string) string {
	if p.Run == nil {
		return ""
	}
	out, err := p.Run(ctx, name, args...)
	if err != nil {
		return ""
	}
	return out
}

// Load1 returns the one minute load average, or -1 when the platform does not report it.
func Load1(ctx context.Context, p Probe) float64 {
	if p.GOOS == "linux" && p.ReadFile != nil {
		if b, err := p.ReadFile("/proc/loadavg"); err == nil {
			return parseFirstFloat(string(b))
		}
		return -1
	}
	if out := runOut(ctx, p, "sysctl", "-n", "vm.loadavg"); out != "" {
		return parseFirstFloat(strings.Trim(out, "{} "))
	}
	return -1
}

func parseFirstFloat(s string) float64 {
	f := strings.Fields(s)
	if len(f) == 0 {
		return -1
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return -1
	}
	return v
}

// ParseCPUInfoModel returns the first "model name" line of /proc/cpuinfo.
func ParseCPUInfoModel(s string) string {
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(k) == "model name" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ParseMemTotal returns MemTotal from /proc/meminfo in bytes.
func ParseMemTotal(s string) uint64 {
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

// ParseOSRelease returns PRETTY_NAME from /etc/os-release.
func ParseOSRelease(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// Summary is a one line description of the host for table headers.
func (h Host) Summary() string {
	parts := []string{fmt.Sprintf("%s/%s", h.OS, h.Arch)}
	if h.OSVersion != "" {
		parts[0] += " " + h.OSVersion
	}
	if h.CPUModel != "" {
		parts = append(parts, h.CPUModel)
	}
	parts = append(parts, fmt.Sprintf("%d CPUs", h.CPUs))
	if h.MemBytes > 0 {
		parts = append(parts, fmt.Sprintf("%d GiB RAM", h.MemBytes>>30))
	}
	if h.Docker.Version != "" {
		d := "Docker " + h.Docker.Version
		if h.Docker.OS != "" {
			d += " (" + h.Docker.OS + ")"
		}
		parts = append(parts, d)
	}
	return strings.Join(parts, ", ")
}
