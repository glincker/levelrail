package fixit

import (
	"os"
	"strconv"
	"strings"
)

// Run modes the control plane can be started in.
const (
	RunSystemd = "systemd"
	RunDocker  = "docker"
	RunDev     = "dev"
)

// HostEnv is how this process is running, read from /proc and the filesystem.
type HostEnv struct {
	Mode        string
	Unit        string
	Root        bool
	TunPresent  bool
	CapNetAdmin bool
	Executable  string
}

// HostReader is the injectable view of the host DetectHostEnv reads.
type HostReader struct {
	ReadFile   func(path string) ([]byte, error)
	Exists     func(path string) bool
	Getenv     func(key string) string
	Geteuid    func() int
	Executable func() string
}

// SystemHostReader reads the real host.
func SystemHostReader() HostReader {
	return HostReader{
		ReadFile: os.ReadFile,
		Exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		Getenv:  os.Getenv,
		Geteuid: os.Geteuid,
		Executable: func() string {
			p, _ := os.Executable()
			return p
		},
	}
}

// DetectHostEnv classifies the run mode and the privileges the process has.
func DetectHostEnv(r HostReader) HostEnv {
	h := HostEnv{Root: r.Geteuid() == 0, TunPresent: r.Exists("/dev/net/tun"), Executable: r.Executable()}
	cgroup := ""
	if b, err := r.ReadFile("/proc/self/cgroup"); err == nil {
		cgroup = string(b)
	}
	switch {
	case r.Exists("/.dockerenv") || strings.Contains(cgroup, "docker") || strings.Contains(cgroup, "kubepods") || strings.Contains(cgroup, "containerd"):
		h.Mode = RunDocker
	case r.Getenv("INVOCATION_ID") != "" || strings.Contains(cgroup, ".service"):
		h.Mode = RunSystemd
		h.Unit = unitFromCgroup(cgroup)
	default:
		h.Mode = RunDev
	}
	if b, err := r.ReadFile("/proc/self/status"); err == nil {
		h.CapNetAdmin = capNetAdmin(string(b))
	}
	return h
}

func unitFromCgroup(cgroup string) string {
	for _, line := range strings.Split(cgroup, "\n") {
		for _, seg := range strings.Split(line, "/") {
			if strings.HasSuffix(seg, ".service") && !strings.HasPrefix(seg, "user@") {
				return strings.TrimSuffix(seg, ".service")
			}
		}
	}
	return ""
}

// capNetAdmin reports bit 12 (CAP_NET_ADMIN) of CapEff.
func capNetAdmin(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, "CapEff:") {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
		return err == nil && v&(1<<12) != 0
	}
	return false
}
