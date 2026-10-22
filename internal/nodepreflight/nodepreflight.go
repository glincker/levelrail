// Package nodepreflight judges the facts a joining node reports about itself
// (Docker, kernel WireGuard, disk, clock, architecture, mesh port) and turns
// each problem into a concrete fix. The agent only gathers facts; judging
// happens on the control plane so thresholds and wording live in one place.
package nodepreflight

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/hostprovider"
	"github.com/GLINCKER/levelrail/internal/network"
)

// Environment variables that tune the judgement. They are read on the
// control plane.
const (
	EnvMinDiskFree   = "APP_NODE_PREFLIGHT_MIN_DISK_FREE_MB"
	EnvMaxClockSkew  = "APP_NODE_PREFLIGHT_MAX_CLOCK_SKEW"
	EnvArchitectures = "APP_NODE_PREFLIGHT_ARCHITECTURES"
)

const (
	defaultMinDiskFreeMB = 5120
	defaultMaxClockSkew  = 30 * time.Second
	defaultArchitectures = "amd64,arm64"
	mib                  = 1 << 20
)

// Status of one check.
const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
	// StatusUnknown means the agent cannot verify it from the node itself.
	StatusUnknown = "unknown"
)

// Check IDs, stable so the dashboard can translate their titles.
const (
	CheckDocker       = "docker"
	CheckArchitecture = "architecture"
	CheckDisk         = "disk"
	CheckClock        = "clock"
	CheckWireGuard    = "wireguard"
	CheckMeshPort     = "mesh_port"
	CheckFirewall     = "firewall"
	CheckControlPlane = "control_plane"
)

// Facts is what the agent measured. AgentTime is its wall clock when it built
// the report.
type Facts struct {
	OS, Arch         string
	DockerOK         bool
	DockerError      string
	DockerVersion    string
	WireGuardKernel  bool
	MeshEnabled      bool
	MeshPort         int
	MeshPortBindable bool
	DiskTotalBytes   uint64
	DiskFreeBytes    uint64
	AgentTime        time.Time
	Provider         string
}

// Config holds the thresholds.
type Config struct {
	MinDiskFreeBytes uint64
	MaxClockSkew     time.Duration
	Architectures    []string
}

// ConfigFromEnv reads the thresholds, falling back to documented defaults on
// a missing or malformed value.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	cfg := Config{MinDiskFreeBytes: defaultMinDiskFreeMB * mib, MaxClockSkew: defaultMaxClockSkew}
	if v, ok := lookup(EnvMinDiskFree); ok {
		if n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64); err == nil {
			cfg.MinDiskFreeBytes = n * mib
		}
	}
	if v, ok := lookup(EnvMaxClockSkew); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
			cfg.MaxClockSkew = d
		}
	}
	archs := defaultArchitectures
	if v, ok := lookup(EnvArchitectures); ok && strings.TrimSpace(v) != "" {
		archs = v
	}
	for _, a := range strings.Split(archs, ",") {
		if a = strings.TrimSpace(a); a != "" {
			cfg.Architectures = append(cfg.Architectures, a)
		}
	}
	return cfg
}

// Check is one judged fact. Fix is empty when the check passed. FixCommand is
// an optional shell command the operator can run on the node.
type Check struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	Fix        string `json:"fix,omitempty"`
	FixCommand string `json:"fix_command,omitempty"`
}

// Summary counts checks by status. Ready is true when nothing failed.
type Summary struct {
	Pass    int  `json:"pass"`
	Warn    int  `json:"warn"`
	Fail    int  `json:"fail"`
	Unknown int  `json:"unknown"`
	Ready   bool `json:"ready"`
}

// Summarize tallies checks.
func Summarize(checks []Check) Summary {
	var s Summary
	for _, c := range checks {
		switch c.Status {
		case StatusPass:
			s.Pass++
		case StatusWarn:
			s.Warn++
		case StatusFail:
			s.Fail++
		default:
			s.Unknown++
		}
	}
	s.Ready = s.Fail == 0
	return s
}

// Evaluate judges facts received at receivedAt. The result order is fixed.
func Evaluate(f Facts, receivedAt time.Time, cfg Config) []Check {
	return []Check{
		{ID: CheckControlPlane, Status: StatusPass, Detail: "the agent reached the control plane and completed the mTLS handshake"},
		dockerCheck(f),
		architectureCheck(f, cfg),
		diskCheck(f, cfg),
		clockCheck(f, receivedAt, cfg),
		wireGuardCheck(f),
		meshPortCheck(f),
		firewallCheck(f),
	}
}

func dockerCheck(f Facts) Check {
	if f.DockerOK {
		detail := "Docker answered on the local socket"
		if f.DockerVersion != "" {
			detail = "Docker " + f.DockerVersion + " answered on the local socket"
		}
		return Check{ID: CheckDocker, Status: StatusPass, Detail: detail}
	}
	detail := "Docker did not answer on the local socket"
	if f.DockerError != "" {
		detail += ": " + f.DockerError
	}
	return Check{
		ID: CheckDocker, Status: StatusFail, Detail: detail,
		Fix:        "Install Docker Engine and make sure the agent can read /var/run/docker.sock (run it as root or add its user to the docker group), then start the daemon.",
		FixCommand: "curl -fsSL https://get.docker.com | sudo sh && sudo systemctl enable --now docker",
	}
}

func architectureCheck(f Facts, cfg Config) Check {
	for _, a := range cfg.Architectures {
		if a == f.Arch && (f.OS == "" || f.OS == "linux") {
			return Check{ID: CheckArchitecture, Status: StatusPass, Detail: f.OS + "/" + f.Arch + " is supported"}
		}
	}
	return Check{
		ID: CheckArchitecture, Status: StatusFail,
		Detail: fmt.Sprintf("%s/%s is not a supported node platform (supported: linux on %s)", f.OS, f.Arch, strings.Join(cfg.Architectures, ", ")),
		Fix:    "Use a Linux machine on a supported CPU architecture. Windows and macOS hosts cannot be nodes.",
	}
}

func diskCheck(f Facts, cfg Config) Check {
	if f.DiskTotalBytes == 0 {
		return Check{ID: CheckDisk, Status: StatusUnknown, Detail: "the agent could not read free disk space on its data directory",
			Fix: "Check that the agent's data directory exists and is on a mounted filesystem."}
	}
	free := f.DiskFreeBytes / mib
	minMiB := cfg.MinDiskFreeBytes / mib
	switch {
	case f.DiskFreeBytes < cfg.MinDiskFreeBytes:
		return Check{ID: CheckDisk, Status: StatusFail,
			Detail:     fmt.Sprintf("%d MiB free, at least %d MiB are needed for images and volumes", free, minMiB),
			Fix:        "Free space or grow the disk. Pruning unused images is the quickest way.",
			FixCommand: "docker system prune --all --force"}
	case f.DiskFreeBytes < cfg.MinDiskFreeBytes*2:
		return Check{ID: CheckDisk, Status: StatusWarn,
			Detail: fmt.Sprintf("%d MiB free, close to the %d MiB minimum", free, minMiB),
			Fix:    "Plan to grow the disk before deploying large images or databases here."}
	}
	return Check{ID: CheckDisk, Status: StatusPass, Detail: fmt.Sprintf("%d MiB free", free)}
}

func clockCheck(f Facts, receivedAt time.Time, cfg Config) Check {
	if f.AgentTime.IsZero() {
		return Check{ID: CheckClock, Status: StatusUnknown, Detail: "the agent did not report its clock"}
	}
	skew := receivedAt.Sub(f.AgentTime)
	abs := skew
	if abs < 0 {
		abs = -abs
	}
	detail := fmt.Sprintf("the node clock differs from the control plane by %s", abs.Round(time.Millisecond))
	fix := "Enable time synchronization on the node so certificate validity and metric timestamps agree."
	cmd := "sudo timedatectl set-ntp true"
	switch {
	case abs > cfg.MaxClockSkew:
		return Check{ID: CheckClock, Status: StatusFail, Detail: detail + ", over the " + cfg.MaxClockSkew.String() + " limit", Fix: fix, FixCommand: cmd}
	case abs > cfg.MaxClockSkew/2:
		return Check{ID: CheckClock, Status: StatusWarn, Detail: detail, Fix: fix, FixCommand: cmd}
	}
	return Check{ID: CheckClock, Status: StatusPass, Detail: detail}
}

func wireGuardCheck(f Facts) Check {
	if f.WireGuardKernel {
		return Check{ID: CheckWireGuard, Status: StatusPass, Detail: "the kernel WireGuard module is loaded"}
	}
	status := StatusWarn
	detail := "the kernel WireGuard module is not loaded, so the mesh would use the slower userspace implementation"
	if f.OS != "" && f.OS != "linux" {
		status = StatusFail
		detail = "WireGuard needs a Linux kernel"
	}
	return Check{
		ID: CheckWireGuard, Status: status, Detail: detail,
		Fix:        "Load the module (kernels 5.6 and newer ship it) or install wireguard-tools for your distribution. Only needed if you enable the mesh.",
		FixCommand: "sudo modprobe wireguard",
	}
}

func meshPortCheck(f Facts) Check {
	port := f.MeshPort
	if port == 0 {
		port = network.DefaultListenPort
	}
	switch {
	case f.MeshEnabled:
		return Check{ID: CheckMeshPort, Status: StatusPass, Detail: fmt.Sprintf("the mesh is running and holds UDP %d", port)}
	case f.MeshPortBindable:
		return Check{ID: CheckMeshPort, Status: StatusPass, Detail: fmt.Sprintf("UDP %d is free on this node", port)}
	}
	return Check{
		ID: CheckMeshPort, Status: StatusWarn,
		Detail:     fmt.Sprintf("UDP %d is already in use on this node", port),
		Fix:        "Stop whatever holds the port (often another WireGuard interface), or choose a different mesh port.",
		FixCommand: fmt.Sprintf("sudo ss -ulpn | grep ':%d '", port),
	}
}

// The agent can only dial out, so it cannot prove that other nodes can reach
// its mesh port from outside. This is reported as unknown with the exact rule
// to open, never as a pass.
func firewallCheck(f Facts) Check {
	port := f.MeshPort
	if port == 0 {
		port = network.DefaultListenPort
	}
	where := "the node's firewall and its cloud security group"
	fix := fmt.Sprintf("Allow inbound UDP %d from the other nodes in %s.", port, where)
	if f.Provider != "" {
		if g := hostprovider.For(hostprovider.ID(f.Provider), hostprovider.SourceMetadata); g.Name != "" {
			fix = fmt.Sprintf("In %s, allow inbound UDP %d from the other nodes (the web rules for TCP 80 and 443 are separate). Steps: %s", g.Name, port, g.DocsURL)
		}
	}
	return Check{
		ID: CheckFirewall, Status: StatusUnknown,
		Detail: fmt.Sprintf("a node only dials out, so inbound reachability of UDP %d between nodes cannot be proven from here; node pair diagnostics report it once the mesh has two nodes", port),
		Fix:    fix,
	}
}
