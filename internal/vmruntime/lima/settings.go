// Package lima renders and validates the Lima template for the managed VM.
package lima

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/vmruntime/guest"
)

// Defaults block (ADR 070 table). Nothing else in this package holds a limit.
const (
	DefaultMemoryMiB        = 3072
	DefaultCPUs             = 2
	DefaultDiskGiB          = 30
	DefaultMemoryMinMiB     = 2048
	DefaultMemoryMaxPercent = 50
	DefaultCPUMin           = 1
	DefaultDiskMinGiB       = 10
	DefaultDiskMaxGiB       = 1024
	DefaultDashboardGuest   = 8080
	DefaultIngressGuest     = 80
	minHostPort             = 1024
	maxPort                 = 65535
	percentMax              = 100
)

// Home subtrees that hold credentials; never shareable even read-only.
var sensitiveHomeDirs = []string{".ssh", ".aws", ".gnupg", ".kube", ".docker", ".config"}

// Share is one explicit host folder exposed to the guest. It is always read-only.
type Share struct {
	HostPath  string
	GuestPath string // Defaults to /mnt/shares/<base name>.
}

// Settings is everything the template needs. Ports are host loopback ports.
type Settings struct {
	MemoryMiB          int
	CPUs               int
	DiskGiB            int
	DashboardPort      int
	IngressPort        int
	DashboardGuestPort int
	IngressGuestPort   int
	Shares             []Share
	Sidecar            guest.Sidecar
	Guest              guest.Options
}

// DefaultSettings returns the ADR defaults with unset (zero) host ports.
func DefaultSettings() Settings {
	return Settings{
		MemoryMiB:          DefaultMemoryMiB,
		CPUs:               DefaultCPUs,
		DiskGiB:            DefaultDiskGiB,
		DashboardGuestPort: DefaultDashboardGuest,
		IngressGuestPort:   DefaultIngressGuest,
		Guest:              guest.DefaultOptions(),
	}
}

// Limits bounds Settings. HomeDir and DataDir guard the share check.
type Limits struct {
	MemoryMinMiB int
	MemoryMaxMiB int
	CPUMin       int
	CPUMax       int
	DiskMinGiB   int
	DiskMaxGiB   int
	HomeDir      string
	DataDir      string
}

// DefaultLimits derives limits from the host. maxPercent comes from
// APP_VM_MEMORY_MAX_PERCENT and falls back to the ADR default when zero.
func DefaultLimits(hostMemoryMiB, hostCPUs, maxPercent int, homeDir, dataDir string) Limits {
	if maxPercent <= 0 {
		maxPercent = DefaultMemoryMaxPercent
	}
	if maxPercent > percentMax {
		maxPercent = percentMax
	}
	return Limits{
		MemoryMinMiB: DefaultMemoryMinMiB,
		MemoryMaxMiB: hostMemoryMiB * maxPercent / percentMax,
		CPUMin:       DefaultCPUMin,
		CPUMax:       hostCPUs,
		DiskMinGiB:   DefaultDiskMinGiB,
		DiskMaxGiB:   DefaultDiskMaxGiB,
		HomeDir:      homeDir,
		DataDir:      dataDir,
	}
}

// Validate checks settings against limits. The first failure is returned.
func (s Settings) Validate(l Limits) error {
	if l.MemoryMaxMiB < l.MemoryMinMiB {
		return fmt.Errorf("host memory cap %d MiB is below the %d MiB floor", l.MemoryMaxMiB, l.MemoryMinMiB)
	}
	if s.MemoryMiB < l.MemoryMinMiB || s.MemoryMiB > l.MemoryMaxMiB {
		return fmt.Errorf("memory %d MiB outside %d to %d MiB", s.MemoryMiB, l.MemoryMinMiB, l.MemoryMaxMiB)
	}
	if s.CPUs < l.CPUMin || s.CPUs > l.CPUMax {
		return fmt.Errorf("cpus %d outside %d to %d", s.CPUs, l.CPUMin, l.CPUMax)
	}
	if s.DiskGiB < l.DiskMinGiB || s.DiskGiB > l.DiskMaxGiB {
		return fmt.Errorf("disk %d GiB outside %d to %d GiB", s.DiskGiB, l.DiskMinGiB, l.DiskMaxGiB)
	}
	if err := checkHostPort("dashboard", s.DashboardPort); err != nil {
		return err
	}
	if err := checkHostPort("ingress", s.IngressPort); err != nil {
		return err
	}
	if s.DashboardPort == s.IngressPort {
		return fmt.Errorf("dashboard and ingress host ports must differ")
	}
	for name, p := range map[string]int{"dashboard": s.DashboardGuestPort, "ingress": s.IngressGuestPort} {
		if p < 1 || p > maxPort {
			return fmt.Errorf("%s guest port %d out of range", name, p)
		}
	}
	if s.DashboardGuestPort == s.IngressGuestPort {
		return fmt.Errorf("dashboard and ingress guest ports must differ")
	}
	seen := map[string]bool{}
	for _, sh := range s.Shares {
		if err := sh.validate(l); err != nil {
			return err
		}
		if seen[sh.guestPath()] {
			return fmt.Errorf("two shares use guest path %s", sh.guestPath())
		}
		seen[sh.guestPath()] = true
	}
	opts := s.Guest
	opts.ExpectedShares = len(s.Shares)
	if err := opts.Validate(); err != nil {
		return err
	}
	return s.Sidecar.Validate()
}

func checkHostPort(name string, p int) error {
	if p < minHostPort || p > maxPort {
		return fmt.Errorf("%s host port %d outside %d to %d", name, p, minHostPort, maxPort)
	}
	return nil
}

func (sh Share) guestPath() string {
	if sh.GuestPath != "" {
		return sh.GuestPath
	}
	return "/mnt/shares/" + filepath.Base(sh.HostPath)
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (sh Share) validate(l Limits) error {
	p := sh.HostPath
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || p == string(filepath.Separator) {
		return fmt.Errorf("share %q must be a clean absolute path below the root", p)
	}
	if g := sh.guestPath(); !strings.HasPrefix(g, "/mnt/") || filepath.Clean(g) != g || strings.ContainsAny(g, " \t\r\n'\"$`\\:") {
		return fmt.Errorf("share guest path %q must be a clean path under /mnt", g)
	}
	// The home directory and any parent of it or of the data directory are off limits.
	for _, guard := range []string{l.HomeDir, l.DataDir} {
		if guard != "" && within(p, guard) {
			return fmt.Errorf("share %q contains a protected directory (%s)", p, guard)
		}
	}
	if l.DataDir != "" && within(l.DataDir, p) {
		return fmt.Errorf("share %q is inside the VM data directory", p)
	}
	if l.HomeDir != "" {
		for _, d := range sensitiveHomeDirs {
			if within(filepath.Join(l.HomeDir, d), p) {
				return fmt.Errorf("share %q is inside a credentials directory", p)
			}
		}
	}
	return nil
}
