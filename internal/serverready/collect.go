package serverready

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	probeTimeout     = 5 * time.Second
	defaultSocket    = "/var/run/docker.sock"
	dnsLookupTimeout = 4 * time.Second
)

var (
	ssUsersPattern = regexp.MustCompile(`\(\("([^"]+)",pid=(\d+)`)
	ufwPortPattern = regexp.MustCompile(`^(\d+)(?::(\d+))?(?:/(?:tcp|udp))?$`)
)

// ParseSS reads `ss -H -ltnp` output into listeners. Lines it cannot read are
// skipped: a half-parsed table must not hide the ports that did parse.
func ParseSS(out string) []Listener {
	var ls []Listener
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || (f[0] != "LISTEN" && !strings.EqualFold(f[0], "listening")) {
			continue
		}
		addr, port, ok := splitHostPort(f[3])
		if !ok {
			continue
		}
		l := Listener{Addr: addr, Port: port}
		if m := ssUsersPattern.FindStringSubmatch(sc.Text()); m != nil {
			l.Process = m[1]
			l.PID, _ = strconv.Atoi(m[2])
		}
		ls = append(ls, l)
	}
	return ls
}

func splitHostPort(s string) (string, int, bool) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return "", 0, false
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return "", 0, false
	}
	return strings.TrimSuffix(strings.TrimPrefix(s[:i], "["), "]"), port, true
}

// ParseUFW reads `ufw status` output.
func ParseUFW(out string) Firewall {
	fw := Firewall{Kind: "ufw", Known: true}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Status:"):
			fw.Active = strings.Contains(line, "active") && !strings.Contains(line, "inactive")
			continue
		case line == "" || !strings.Contains(line, "ALLOW"):
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		spec := fields[0]
		if strings.Contains(strings.ToLower(line), "full") || strings.EqualFold(fields[0], "WWW") || strings.HasPrefix(strings.ToLower(line), "nginx") || strings.HasPrefix(strings.ToLower(line), "apache") {
			fw.Allowed = append(fw.Allowed, 80, 443)
			continue
		}
		for _, part := range strings.Split(spec, ",") {
			m := ufwPortPattern.FindStringSubmatch(part)
			if m == nil {
				continue
			}
			lo, _ := strconv.Atoi(m[1])
			hi := lo
			if m[2] != "" {
				hi, _ = strconv.Atoi(m[2])
			}
			for _, p := range []int{80, 443} {
				if p >= lo && p <= hi {
					fw.Allowed = append(fw.Allowed, p)
				}
			}
		}
	}
	return fw
}

// ParseFirewalld reads firewall-cmd --list-ports and --list-services output.
func ParseFirewalld(ports, services string) Firewall {
	fw := Firewall{Kind: "firewalld", Known: true, Active: true}
	for _, p := range strings.Fields(ports) {
		if n, err := strconv.Atoi(strings.SplitN(p, "/", 2)[0]); err == nil {
			fw.Allowed = append(fw.Allowed, n)
		}
	}
	for _, s := range strings.Fields(services) {
		switch s {
		case "http":
			fw.Allowed = append(fw.Allowed, 80)
		case "https":
			fw.Allowed = append(fw.Allowed, 443)
		}
	}
	return fw
}

// Probes are the host reads Collect performs, injected so tests need no host.
type Probes struct {
	SS            func(ctx context.Context) (string, error)
	UFW           func(ctx context.Context) (string, error)
	Firewalld     func(ctx context.Context) (ports, services string, err error)
	DockerInfo    func(ctx context.Context) (DockerInfo, error)
	Published     func(ctx context.Context) ([]Listener, error)
	SocketMode    func(path string) (os.FileMode, error)
	DiskFreeGB    func(path string) (float64, error)
	MemoryMB      func() (int, error)
	LookupHost    func(ctx context.Context, host string) ([]string, error)
	LocalIPs      func() []string
	HasSystemd    func() bool
	DockerPresent func() bool
}

// DockerInfo is what the engine says about itself.
type DockerInfo struct {
	Version  string
	Rootless bool
}

// Request is what to look at.
type Request struct {
	Domain        string
	DataDir       string
	SelfProcesses []string
	SocketPath    string
}

// Collect gathers Facts. A probe that fails leaves its field at the zero
// value and, for Docker, records the error: a readiness report must never
// itself fail because one tool is missing.
func Collect(ctx context.Context, p Probes, req Request) Facts {
	f := Facts{OS: runtime.GOOS, Arch: runtime.GOARCH, SelfProcesses: req.SelfProcesses, Domain: strings.ToLower(strings.TrimSpace(req.Domain))}
	if p.HasSystemd != nil {
		f.Systemd = p.HasSystemd()
	}
	if p.SS != nil {
		if out, err := p.SS(ctx); err == nil {
			f.Listeners = ParseSS(out)
		}
	}
	if p.Published != nil {
		if pub, err := p.Published(ctx); err == nil {
			f.Listeners = mergePublished(f.Listeners, pub)
		}
	}
	if p.DockerPresent != nil {
		f.DockerPresent = p.DockerPresent()
	}
	if p.DockerInfo != nil {
		info, err := p.DockerInfo(ctx)
		switch {
		case err != nil:
			f.DockerError = err.Error()
		default:
			f.DockerPresent = true
			f.DockerVersion, f.DockerRootless = info.Version, info.Rootless
		}
	}
	f.DockerSocketPath = req.SocketPath
	if f.DockerSocketPath == "" {
		f.DockerSocketPath = defaultSocket
	}
	if p.SocketMode != nil {
		if mode, err := p.SocketMode(f.DockerSocketPath); err == nil {
			f.DockerSocketWorldWritable = mode&0o002 != 0
		}
	}
	f.DiskPath = req.DataDir
	if p.DiskFreeGB != nil && req.DataDir != "" {
		if gb, err := p.DiskFreeGB(req.DataDir); err == nil {
			f.DiskFreeGB = gb
		}
	}
	if p.MemoryMB != nil {
		if mb, err := p.MemoryMB(); err == nil {
			f.MemoryMB = mb
		}
	}
	f.Firewall = collectFirewall(ctx, p)
	if p.LocalIPs != nil {
		f.LocalIPs = p.LocalIPs()
	}
	if f.Domain != "" && p.LookupHost != nil {
		ips, err := p.LookupHost(ctx, f.Domain)
		if err != nil {
			f.DNSError = err.Error()
		}
		f.DomainIPs = ips
	}
	return f
}

func collectFirewall(ctx context.Context, p Probes) Firewall {
	if p.UFW != nil {
		if out, err := p.UFW(ctx); err == nil && strings.Contains(out, "Status:") {
			return ParseUFW(out)
		}
	}
	if p.Firewalld != nil {
		if ports, services, err := p.Firewalld(ctx); err == nil {
			return ParseFirewalld(ports, services)
		}
	}
	return Firewall{}
}

// mergePublished attaches container identity to listeners owned by the
// Docker userland proxy, and adds published ports ss could not see.
func mergePublished(ls, pub []Listener) []Listener {
	for _, c := range pub {
		matched := false
		for i := range ls {
			if ls[i].Port == c.Port && (ls[i].Process == "docker-proxy" || ls[i].Process == "") {
				ls[i].Container, ls[i].Image = c.Container, c.Image
				matched = true
			}
		}
		if !matched {
			ls = append(ls, c)
		}
	}
	return ls
}

// HostProbes returns the real probes. DockerInfo and Published are left nil
// for the caller, which already holds an engine client.
func HostProbes() Probes {
	return Probes{
		SS:  func(ctx context.Context) (string, error) { return run(ctx, "ss", "-H", "-ltnp") },
		UFW: func(ctx context.Context) (string, error) { return run(ctx, "ufw", "status") },
		Firewalld: func(ctx context.Context) (string, string, error) {
			if _, err := run(ctx, "firewall-cmd", "--state"); err != nil {
				return "", "", err
			}
			ports, _ := run(ctx, "firewall-cmd", "--list-ports")
			services, _ := run(ctx, "firewall-cmd", "--list-services")
			return ports, services, nil
		},
		SocketMode: func(path string) (os.FileMode, error) {
			st, err := os.Stat(path)
			if err != nil {
				return 0, err
			}
			return st.Mode().Perm(), nil
		},
		DiskFreeGB: func(path string) (float64, error) {
			var st syscall.Statfs_t
			if err := syscall.Statfs(path, &st); err != nil {
				return 0, err
			}
			return float64(st.Bavail) * float64(st.Bsize) / (1 << 30), nil //nolint:gosec // statfs fields are non-negative
		},
		MemoryMB: memoryMB,
		LookupHost: func(ctx context.Context, host string) ([]string, error) {
			ctx, cancel := context.WithTimeout(ctx, dnsLookupTimeout)
			defer cancel()
			return net.DefaultResolver.LookupHost(ctx, host)
		},
		LocalIPs: localIPs,
		HasSystemd: func() bool {
			_, err := os.Stat("/run/systemd/system")
			return err == nil
		},
		DockerPresent: func() bool {
			_, err := exec.LookPath("docker")
			return err == nil
		},
	}
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // fixed read-only diagnostic programs
	return string(out), err
}

func memoryMB() (int, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				kb, err := strconv.Atoi(fields[0])
				return kb / 1024, err
			}
		}
	}
	return 0, sc.Err()
}

func localIPs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			out = append(out, ipn.IP.String())
		}
	}
	return out
}
