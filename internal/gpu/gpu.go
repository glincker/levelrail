// Package gpu detects NVIDIA GPUs on a node by querying nvidia-smi and
// the Docker daemon's registered runtimes. It never shells out to the
// docker CLI.
package gpu

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// InstallHint is the operator-facing remedy when GPUs exist but Docker
// has no nvidia runtime.
const InstallHint = "Install nvidia-container-toolkit on the node (Ubuntu: add NVIDIA's apt repository, apt-get install -y nvidia-container-toolkit, then nvidia-ctk runtime configure --runtime=docker and systemctl restart docker)."

// DriverHint is the operator-facing remedy when no NVIDIA driver answers.
const DriverHint = "No NVIDIA driver detected. On Ubuntu run ubuntu-drivers install (or apt-get install nvidia-driver-XXX), then reboot."

const smiTimeout = 10 * time.Second

// Device is one GPU as reported by nvidia-smi.
type Device struct {
	Index              int
	UUID               string
	Name               string
	VRAMTotalMiB       int64
	VRAMUsedMiB        int64
	UtilizationPercent int
}

// Info is a node's GPU capability snapshot.
type Info struct {
	Present          bool
	DriverVersion    string
	Devices          []Device
	RuntimeInstalled bool
}

// Count returns the number of GPUs.
func (i Info) Count() int { return len(i.Devices) }

// TotalVRAMMiB sums VRAM across devices.
func (i Info) TotalVRAMMiB() int64 {
	var total int64
	for _, d := range i.Devices {
		total += d.VRAMTotalMiB
	}
	return total
}

// Usable reports whether a container can be given a GPU on this node.
func (i Info) Usable() bool { return i.Present && i.RuntimeInstalled }

// Hint returns remediation text when the node has a GPU problem, or
// empty when there is nothing to fix (including nodes with no GPU).
func (i Info) Hint() string {
	if i.Present && !i.RuntimeInstalled {
		return InstallHint
	}
	return ""
}

// Runner runs a host command, returning its stdout.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// RuntimeLister reports the OCI runtimes registered with Docker.
type RuntimeLister interface {
	RuntimeNames(ctx context.Context) ([]string, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, smiTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // fixed binary name and static args from this package
	if err != nil {
		return nil, fmt.Errorf("gpu: run %s: %w", name, err)
	}
	return out, nil
}

var smiArgs = []string{
	"--query-gpu=index,uuid,name,memory.total,memory.used,utilization.gpu,driver_version",
	"--format=csv,noheader,nounits",
}

// Detect returns the node's GPU snapshot. A missing nvidia-smi or a
// failing query means no GPU, not an error: most nodes have none.
func Detect(ctx context.Context, r Runner, rl RuntimeLister) Info {
	out, err := r.Run(ctx, "nvidia-smi", smiArgs...)
	if err != nil {
		return Info{}
	}
	driver, devices, err := ParseSMI(string(out))
	if err != nil || len(devices) == 0 {
		return Info{}
	}
	info := Info{Present: true, DriverVersion: driver, Devices: devices}
	if rl != nil {
		if names, err := rl.RuntimeNames(ctx); err == nil {
			for _, n := range names {
				if strings.EqualFold(n, "nvidia") {
					info.RuntimeInstalled = true
				}
			}
		}
	}
	return info
}

// ParseSMI parses the CSV emitted by nvidia-smi with smiArgs.
func ParseSMI(out string) (driver string, devices []Device, err error) {
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(out)))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = 7
	rows, err := r.ReadAll()
	if err != nil {
		if strings.TrimSpace(out) == "" {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("gpu: parse nvidia-smi output: %w", err)
	}
	for _, row := range rows {
		d, drv, perr := parseRow(row)
		if perr != nil {
			return "", nil, perr
		}
		driver = drv
		devices = append(devices, d)
	}
	return driver, devices, nil
}

var errBadRow = errors.New("gpu: malformed nvidia-smi row")

func parseRow(row []string) (Device, string, error) {
	idx, err1 := strconv.Atoi(strings.TrimSpace(row[0]))
	total, err2 := strconv.ParseInt(strings.TrimSpace(row[3]), 10, 64)
	used, err3 := strconv.ParseInt(strings.TrimSpace(row[4]), 10, 64)
	if err := errors.Join(err1, err2, err3); err != nil {
		return Device{}, "", fmt.Errorf("%w: %v", errBadRow, err)
	}
	util, _ := strconv.Atoi(strings.TrimSpace(row[5])) // "[N/A]" on some virtualized GPUs
	return Device{
		Index:              idx,
		UUID:               strings.TrimSpace(row[1]),
		Name:               strings.TrimSpace(row[2]),
		VRAMTotalMiB:       total,
		VRAMUsedMiB:        used,
		UtilizationPercent: util,
	}, strings.TrimSpace(row[6]), nil
}
