package gpu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Finding statuses, matching the doctor report's vocabulary.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// Attach modes a host can use for GPU containers.
const (
	AttachCDI    = "cdi"
	AttachLegacy = "legacy"
	AttachNone   = "none"
)

const (
	// CDIGenerateCommand writes the NVIDIA CDI spec that CDI attach needs.
	CDIGenerateCommand = "sudo nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml"
	// CDIEnableHint is the remedy when a spec exists but Docker lists no CDI devices.
	CDIEnableHint = `Docker 28.3 or newer discovers CDI specs by default; on older Docker set {"features": {"cdi": true}} in /etc/docker/daemon.json and restart Docker.`

	envVRAMWarnPercent = "APP_GPU_DOCTOR_VRAM_WARN_PERCENT"
)

// cdiSpecGlobs are the directories CDI specs are read from.
var cdiSpecGlobs = []string{"/etc/cdi/*", "/var/run/cdi/*"}

// DockerFacts is what the diagnosis asks the Docker daemon.
type DockerFacts interface {
	RuntimeNames(ctx context.Context) ([]string, error)
	CDIDevices(ctx context.Context) ([]string, error)
}

// Finding is one diagnosed aspect of a host's GPU setup.
type Finding struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// HostDiagnosis is the detailed GPU picture of one host.
type HostDiagnosis struct {
	Info           Info
	ToolkitVersion string
	CDISpecFiles   []string
	CDIDevices     []string
	Attach         string
	Findings       []Finding
}

// DiagnoseHost inspects this host's GPU stack: driver, per-GPU memory, the
// NVIDIA container toolkit, CDI specs and how Docker can attach GPUs.
// glob may be nil to use the real filesystem.
func DiagnoseHost(ctx context.Context, r Runner, d DockerFacts, glob func(pattern string) []string) HostDiagnosis {
	if glob == nil {
		glob = func(p string) []string { m, _ := filepath.Glob(p); return m }
	}
	info := Detect(ctx, r, d)
	out := HostDiagnosis{Info: info}
	if !info.Present {
		out.Attach = AttachNone
		out.Findings = []Finding{{Code: "driver", Name: "NVIDIA driver", Status: StatusFail, Message: "nvidia-smi did not report any GPU", Fix: "ubuntu-drivers install, then reboot"}}
		return out
	}
	out.Findings = append(out.Findings, Finding{Code: "driver", Name: "NVIDIA driver", Status: StatusOK,
		Message: fmt.Sprintf("driver %s, %d GPU(s)", info.DriverVersion, info.Count())})
	out.Findings = append(out.Findings, memoryFindings(info)...)

	if b, err := r.Run(ctx, "nvidia-ctk", "--version"); err == nil {
		out.ToolkitVersion = firstLine(string(b))
		out.Findings = append(out.Findings, Finding{Code: "toolkit", Name: "NVIDIA container toolkit", Status: StatusOK, Message: out.ToolkitVersion})
	} else {
		out.Findings = append(out.Findings, Finding{Code: "toolkit", Name: "NVIDIA container toolkit", Status: StatusWarn,
			Message: "nvidia-ctk was not found on the PATH", Fix: "sudo apt-get install -y nvidia-container-toolkit"})
	}

	seen := map[string]bool{}
	for _, g := range cdiSpecGlobs {
		for _, f := range glob(g) {
			if !seen[f] && isNvidiaSpec(f) {
				seen[f] = true
				out.CDISpecFiles = append(out.CDISpecFiles, f)
			}
		}
	}
	legacy := false
	if d != nil {
		out.CDIDevices, _ = d.CDIDevices(ctx)
		names, _ := d.RuntimeNames(ctx)
		for _, n := range names {
			legacy = legacy || strings.EqualFold(n, "nvidia")
		}
	}
	out.Attach = attachMode(out, legacy)
	out.Findings = append(out.Findings, attachFindings(legacy, out)...)
	return out
}

func attachMode(h HostDiagnosis, legacy bool) string {
	switch {
	case len(h.CDIDevices) > 0:
		return AttachCDI
	case legacy:
		return AttachLegacy
	}
	return AttachNone
}

func attachFindings(legacy bool, h HostDiagnosis) []Finding {
	var fs []Finding
	switch {
	case len(h.CDIDevices) > 0:
		fs = append(fs, Finding{Code: "cdi", Name: "CDI", Status: StatusOK,
			Message: fmt.Sprintf("Docker lists %d NVIDIA CDI device(s); new GPU containers attach through CDI", len(h.CDIDevices))})
	case len(h.CDISpecFiles) > 0:
		fs = append(fs, Finding{Code: "cdi", Name: "CDI", Status: StatusWarn,
			Message: "a CDI spec exists (" + strings.Join(h.CDISpecFiles, ", ") + ") but Docker lists no NVIDIA CDI devices", Fix: CDIEnableHint})
	default:
		fs = append(fs, Finding{Code: "cdi", Name: "CDI", Status: StatusOK,
			Message: "no CDI spec; GPU containers use the legacy nvidia runtime. To use CDI, generate a spec: " + CDIGenerateCommand})
	}
	if legacy {
		fs = append(fs, Finding{Code: "runtime", Name: "Docker nvidia runtime", Status: StatusOK, Message: "registered (legacy attach available)"})
	} else if len(h.CDIDevices) == 0 {
		fs = append(fs, Finding{Code: "runtime", Name: "Docker nvidia runtime", Status: StatusWarn, Message: "not registered with Docker", Fix: InstallCommand})
	}
	if h.Attach == AttachNone {
		fs = append(fs, Finding{Code: "attach", Name: "GPU attach", Status: StatusFail,
			Message: "Docker can attach GPUs neither through CDI nor the nvidia runtime, so containers cannot use them", Fix: InstallCommand})
	} else {
		fs = append(fs, Finding{Code: "attach", Name: "GPU attach", Status: StatusOK, Message: "containers attach GPUs through " + h.Attach})
	}
	return fs
}

func memoryFindings(info Info) []Finding {
	warnAt := 90.0
	if v, err := strconv.ParseFloat(os.Getenv(envVRAMWarnPercent), 64); err == nil && v > 0 && v <= 100 {
		warnAt = v
	}
	var fs []Finding
	for _, d := range info.Devices {
		f := Finding{Code: fmt.Sprintf("gpu%d", d.Index), Name: fmt.Sprintf("GPU %d memory", d.Index), Status: StatusOK,
			Message: fmt.Sprintf("%s: %s used of %s", d.Name, mibString(d.VRAMUsedMiB), mibString(d.VRAMTotalMiB))}
		if d.VRAMTotalMiB > 0 && float64(d.VRAMUsedMiB)/float64(d.VRAMTotalMiB)*100 >= warnAt {
			f.Status = StatusWarn
			f.Message += fmt.Sprintf(" (at least %.0f%% in use; a new model may not fit)", warnAt)
		}
		fs = append(fs, f)
	}
	return fs
}

func mibString(mib int64) string { return fmt.Sprintf("%.1f GiB", float64(mib)/1024) }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// isNvidiaSpec reports whether a CDI spec file declares NVIDIA GPU devices.
func isNvidiaSpec(path string) bool {
	if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".json") {
		return false
	}
	b, err := os.ReadFile(path) //nolint:gosec // path comes from a fixed CDI directory glob
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "nvidia.com/gpu")
}
