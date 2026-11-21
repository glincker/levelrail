package macosrt

import (
	"fmt"
	"strconv"
	"strings"
)

// Check statuses match the doctor report vocabulary.
const (
	StatusOK      = "ok"
	StatusWarn    = "warn"
	StatusFail    = "fail"
	StatusUnknown = "unknown"
)

// Result is one macOS readiness check.
type Result struct {
	Code    string
	Name    string
	Status  string
	Message string
	Fix     string
}

// Deps are the host probes, injected so tests need no real Mac.
type Deps struct {
	GOOS      string
	GOARCH    string
	LookPath  func(string) (string, error)
	Sysctl    func(name string) (string, error)
	Version   func() (string, error)
	FreeBytes func(dir string) (int64, error)
	PortFree  func(port int) bool
	DataDir   string
	// MinFreeGiB and MinMacOSMajor come from env at the call site.
	MinFreeGiB    int
	MinMacOSMajor int
	Ports         []int
}

const bytesPerGiB = int64(1) << 30

// Run returns every check. Off macOS it returns a single warn, never a failure.
func Run(d Deps) []Result {
	if d.GOOS != "darwin" {
		return []Result{{Code: "macos_host", Name: "macOS host", Status: StatusWarn,
			Message: "this host is not macOS, the checks do not apply"}}
	}
	out := []Result{archCheck(d), versionCheck(d), limaCheck(d), vzCheck(d), diskCheck(d)}
	for _, p := range d.Ports {
		out = append(out, portCheck(d, p))
	}
	return out
}

func archCheck(d Deps) Result {
	r := Result{Code: "macos_arch", Name: "CPU architecture"}
	switch d.GOARCH {
	case "arm64", "amd64":
		r.Status, r.Message = StatusOK, d.GOARCH
	default:
		r.Status, r.Message = StatusFail, "unsupported architecture "+d.GOARCH
	}
	return r
}

func versionCheck(d Deps) Result {
	r := Result{Code: "macos_version", Name: "macOS version"}
	v, err := d.Version()
	if err != nil {
		r.Status, r.Message = StatusUnknown, "could not read the macOS version"
		return r
	}
	major, _ := strconv.Atoi(strings.SplitN(strings.TrimSpace(v), ".", 2)[0])
	if major < d.MinMacOSMajor {
		r.Status = StatusFail
		r.Message = fmt.Sprintf("macOS %s is older than %d, the VM framework needs a newer release", v, d.MinMacOSMajor)
		r.Fix = "update macOS"
		return r
	}
	r.Status, r.Message = StatusOK, v
	return r
}

func limaCheck(d Deps) Result {
	r := Result{Code: "macos_lima", Name: "Lima"}
	p, err := d.LookPath("limactl")
	if err != nil {
		r.Status, r.Message = StatusFail, "limactl not found"
		r.Fix = "brew install lima"
		return r
	}
	r.Status, r.Message = StatusOK, p
	return r
}

// vzCheck asks the kernel for hypervisor support, which the vz driver needs.
func vzCheck(d Deps) Result {
	r := Result{Code: "macos_vz", Name: "Virtualization support"}
	v, err := d.Sysctl("kern.hv_support")
	switch {
	case err != nil:
		r.Status, r.Message = StatusUnknown, "could not read kern.hv_support"
	case strings.TrimSpace(v) == "1":
		r.Status, r.Message = StatusOK, "hypervisor available"
	default:
		r.Status, r.Message = StatusFail, "the hypervisor is not available on this Mac"
		r.Fix = "run on a Mac with virtualization enabled, not inside another VM without nested support"
	}
	return r
}

func diskCheck(d Deps) Result {
	r := Result{Code: "macos_disk", Name: "Disk space for the VM"}
	free, err := d.FreeBytes(d.DataDir)
	if err != nil {
		r.Status, r.Message = StatusUnknown, "could not read free disk space"
		return r
	}
	need := int64(d.MinFreeGiB) * bytesPerGiB
	g := free / bytesPerGiB
	if free < need {
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("%d GiB free, below the %d GiB needed to create the VM disk", g, d.MinFreeGiB)
		r.Fix = "free disk space or lower the VM disk with: levelrail vm set disk <GiB>"
		return r
	}
	r.Status, r.Message = StatusOK, fmt.Sprintf("%d GiB free", g)
	return r
}

func portCheck(d Deps, port int) Result {
	r := Result{Code: "macos_port_" + strconv.Itoa(port), Name: fmt.Sprintf("Loopback port %d", port)}
	if d.PortFree(port) {
		r.Status, r.Message = StatusOK, "available"
		return r
	}
	r.Status, r.Message = StatusWarn, "in use, expected if the VM is already running"
	r.Fix = "change it with: levelrail vm set dashboard-port <port> (or ingress-port)"
	return r
}
