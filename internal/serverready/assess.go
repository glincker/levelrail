// Package serverready reports whether a server can run the control plane and
// in which mode. Assess is pure over Facts, so each proxy and port situation
// is a table fixture; Collect gathers real Facts.
package serverready

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

// Status of one check.
const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
	StatusInfo = "info"
)

// Install modes.
const (
	ModeOwnPorts    = "own_ports"
	ModeBehindProxy = "behind_proxy"
)

// Check IDs.
const (
	CheckOS            = "os"
	CheckDocker        = "docker"
	CheckDockerRootles = "docker_rootless"
	CheckDockerSocket  = "docker_socket"
	CheckPort80        = "port_80"
	CheckPort443       = "port_443"
	CheckDisk          = "disk"
	CheckMemory        = "memory"
	CheckFirewall      = "firewall"
	CheckDNS           = "dns"
)

// Proxy kinds recognised by process or container image name.
const (
	ProxyTraefik = "traefik"
	ProxyNginx   = "nginx"
	ProxyCaddy   = "caddy"
	ProxyApache  = "apache"
	ProxyHAProxy = "haproxy"
	ProxyOther   = "unknown"
)

// Listener is one TCP listening socket and who owns it.
type Listener struct {
	Addr    string `json:"addr"`
	Port    int    `json:"port"`
	Process string `json:"process,omitempty"`
	PID     int    `json:"pid,omitempty"`
	// Container and Image are set when a Docker container publishes the port.
	Container string `json:"container,omitempty"`
	Image     string `json:"image,omitempty"`
}

// Firewall describes the host firewall as far as it can be read.
type Firewall struct {
	// Kind is ufw, firewalld or "" when none was found.
	Kind    string `json:"kind"`
	Active  bool   `json:"active"`
	Known   bool   `json:"known"`
	Allowed []int  `json:"allowed,omitempty"`
}

// Facts is everything Assess needs, gathered once.
type Facts struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Systemd bool   `json:"systemd"`

	Listeners []Listener `json:"listeners"`
	// SelfProcesses are process names that mean "this product is already
	// running here", so its own ports are not a conflict.
	SelfProcesses []string `json:"-"`

	DockerPresent  bool   `json:"docker_present"`
	DockerVersion  string `json:"docker_version,omitempty"`
	DockerRootless bool   `json:"docker_rootless"`
	DockerError    string `json:"docker_error,omitempty"`
	// DockerSocketWorldWritable and DockerTCPExposed flag an exposed daemon.
	DockerSocketPath          string `json:"docker_socket_path,omitempty"`
	DockerSocketWorldWritable bool   `json:"docker_socket_world_writable"`

	DiskFreeGB float64 `json:"disk_free_gb"`
	DiskPath   string  `json:"disk_path,omitempty"`
	MemoryMB   int     `json:"memory_mb"`

	Firewall Firewall `json:"firewall"`

	Domain    string   `json:"domain,omitempty"`
	DomainIPs []string `json:"domain_ips,omitempty"`
	DNSError  string   `json:"dns_error,omitempty"`
	LocalIPs  []string `json:"local_ips,omitempty"`
}

// Thresholds are the minimums, supplied by the caller from configuration.
type Thresholds struct {
	MinDiskGB       float64
	MinMemoryMB     int
	MinDockerMajor  int
	HTTPPort        int
	HTTPSPort       int
	InstallerPrefix string
}

// Check is one result.
type Check struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Report is the assessment.
type Report struct {
	Checks []Check `json:"checks"`
	// Mode is the recommended install mode.
	Mode string `json:"mode"`
	// Proxy is the reverse proxy found on 80/443, "" when none.
	Proxy string `json:"proxy,omitempty"`
	// Holders names who owns each contested port.
	Holders map[int]string `json:"holders,omitempty"`
	// NextStep is the exact command or action to take now.
	NextStep string `json:"next_step"`
	Blocked  bool   `json:"blocked"`
	Summary  string `json:"summary"`
}

// ClassifyProxy names the proxy family of a process or image, "" if neither
// looks like a web server or proxy.
func ClassifyProxy(process, image string) string {
	s := strings.ToLower(process + " " + image)
	switch {
	case strings.Contains(s, "traefik"):
		return ProxyTraefik
	case strings.Contains(s, "nginx"), strings.Contains(s, "openresty"):
		return ProxyNginx
	case strings.Contains(s, "caddy"):
		return ProxyCaddy
	case strings.Contains(s, "apache"), strings.Contains(s, "httpd"):
		return ProxyApache
	case strings.Contains(s, "haproxy"):
		return ProxyHAProxy
	}
	return ""
}

func (f Facts) isSelf(l Listener) bool {
	for _, p := range f.SelfProcesses {
		if p != "" && strings.EqualFold(l.Process, p) {
			return true
		}
	}
	return false
}

// holder picks the listener that owns port, preferring one that is not us.
func (f Facts) holder(port int) (Listener, bool) {
	var found *Listener
	for i := range f.Listeners {
		l := f.Listeners[i]
		if l.Port != port {
			continue
		}
		if f.isSelf(l) {
			return l, true
		}
		if found == nil {
			found = &f.Listeners[i]
		}
	}
	if found == nil {
		return Listener{}, false
	}
	return *found, true
}

func describe(l Listener) string {
	switch {
	case l.Container != "":
		return fmt.Sprintf("container %s (image %s)", l.Container, l.Image)
	case l.Process != "":
		if l.PID > 0 {
			return fmt.Sprintf("%s (pid %d)", l.Process, l.PID)
		}
		return l.Process
	}
	return "an unidentified process (run as root with ss installed to see which)"
}

// Assess turns facts into a report and a recommended mode.
func Assess(f Facts, th Thresholds) Report {
	if th.HTTPPort == 0 {
		th.HTTPPort = 80
	}
	if th.HTTPSPort == 0 {
		th.HTTPSPort = 443
	}
	if th.MinDockerMajor == 0 {
		th.MinDockerMajor = 24
	}
	r := Report{Holders: map[int]string{}}
	r.Checks = append(r.Checks, osCheck(f))
	r.Checks = append(r.Checks, dockerChecks(f, th)...)
	portChecks, proxy, contested := portChecks(f, th, &r)
	r.Checks = append(r.Checks, portChecks...)
	r.Proxy = proxy
	r.Checks = append(r.Checks, diskCheck(f, th), memoryCheck(f, th), firewallCheck(f, th))
	if f.Domain != "" {
		r.Checks = append(r.Checks, dnsCheck(f))
	}
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			r.Blocked = true
		}
	}
	if contested {
		r.Mode = ModeBehindProxy
	} else {
		r.Mode = ModeOwnPorts
	}
	r.NextStep, r.Summary = recommend(r, f, th)
	return r
}

func osCheck(f Facts) Check {
	c := Check{ID: CheckOS, Name: "Operating system"}
	switch {
	case f.OS != "linux":
		c.Status, c.Detail = StatusFail, f.OS+" is not supported, the control plane runs on Linux servers"
	case !f.Systemd:
		c.Status, c.Detail, c.Fix = StatusFail, "systemd is not the init system", "Use a systemd based distribution (Ubuntu, Debian, Fedora, Rocky)."
	case f.Arch != "amd64" && f.Arch != "arm64":
		c.Status, c.Detail = StatusFail, "architecture "+f.Arch+" is not supported (amd64 and arm64 are)"
	default:
		c.Status, c.Detail = StatusPass, "linux/"+f.Arch+" with systemd"
	}
	return c
}

func majorOf(v string) int {
	n, _ := strconv.Atoi(strings.SplitN(strings.TrimPrefix(v, "v"), ".", 2)[0])
	return n
}

func dockerChecks(f Facts, th Thresholds) []Check {
	d := Check{ID: CheckDocker, Name: "Docker Engine"}
	switch {
	case !f.DockerPresent:
		d.Status, d.Detail = StatusWarn, "Docker is not installed"
		d.Fix = "The installer installs it for you through get.docker.com."
	case f.DockerError != "":
		d.Status, d.Detail, d.Fix = StatusFail, "Docker is installed but not answering: "+f.DockerError, "Start it with: sudo systemctl enable --now docker"
	case majorOf(f.DockerVersion) > 0 && majorOf(f.DockerVersion) < th.MinDockerMajor:
		d.Status = StatusFail
		d.Detail = fmt.Sprintf("Docker %s is older than the supported %d", f.DockerVersion, th.MinDockerMajor)
		d.Fix = "Upgrade Docker Engine: https://docs.docker.com/engine/install/"
	default:
		d.Status, d.Detail = StatusPass, "Docker "+f.DockerVersion
	}
	out := []Check{d}

	if f.DockerPresent && f.DockerError == "" {
		rl := Check{ID: CheckDockerRootles, Name: "Docker isolation"}
		if f.DockerRootless {
			rl.Status = StatusWarn
			rl.Detail = "Docker runs rootless: it cannot publish ports 80 and 443 or reach host paths without extra setup"
			rl.Fix = "Allow low ports: echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/99-lowports.conf && sudo sysctl --system"
		} else {
			rl.Status, rl.Detail = StatusPass, "Docker runs with a root daemon"
		}
		out = append(out, rl)
	}

	sock := Check{ID: CheckDockerSocket, Name: "Docker socket exposure"}
	switch {
	case dockerTCPExposed(f):
		sock.Status = StatusFail
		sock.Detail = "the Docker API listens on a network address without TLS (port 2375): anyone who can reach it controls this server"
		sock.Fix = "Remove the tcp:// host from the daemon's hosts setting (/etc/docker/daemon.json or its systemd drop-in) and restart Docker."
	case f.DockerSocketWorldWritable:
		sock.Status = StatusWarn
		sock.Detail = fmt.Sprintf("%s is writable by every local user, which is root access to the host", orDefault(f.DockerSocketPath, "/var/run/docker.sock"))
		sock.Fix = "Restrict it: sudo chmod 660 " + orDefault(f.DockerSocketPath, "/var/run/docker.sock") + " (owner root, group docker)."
	default:
		sock.Status, sock.Detail = StatusPass, "the Docker API is not exposed on the network"
	}
	return append(out, sock)
}

func dockerTCPExposed(f Facts) bool {
	for _, l := range f.Listeners {
		if l.Port != 2375 {
			continue
		}
		if ip := net.ParseIP(strings.Trim(l.Addr, "[]")); ip != nil && ip.IsLoopback() {
			continue
		}
		return true
	}
	return false
}

// portChecks reports ports 80 and 443. contested is true when either is held
// by something other than this product, which selects behind-proxy mode.
func portChecks(f Facts, th Thresholds, r *Report) (checks []Check, proxy string, contested bool) {
	for _, p := range []struct {
		id   string
		port int
	}{{CheckPort80, th.HTTPPort}, {CheckPort443, th.HTTPSPort}} {
		c := Check{ID: p.id, Name: fmt.Sprintf("Port %d", p.port)}
		l, held := f.holder(p.port)
		switch {
		case !held:
			c.Status, c.Detail = StatusPass, "free"
		case f.isSelf(l):
			c.Status, c.Detail = StatusPass, "in use by this installation"
		default:
			contested = true
			kind := ClassifyProxy(l.Process, l.Image)
			r.Holders[p.port] = describe(l)
			if kind != "" && proxy == "" {
				proxy = kind
			}
			c.Status = StatusWarn
			if kind != "" {
				c.Detail = fmt.Sprintf("held by %s, a %s reverse proxy", describe(l), kind)
				c.Fix = "Install behind it: the installer picks this mode automatically and your proxy keeps terminating TLS."
			} else {
				c.Detail = "held by " + describe(l)
				c.Fix = "Stop that service, or install behind it as a reverse proxy target (automatic with the installer)."
			}
		}
		checks = append(checks, c)
	}
	return checks, proxy, contested
}

func diskCheck(f Facts, th Thresholds) Check {
	c := Check{ID: CheckDisk, Name: "Free disk space"}
	floor := th.MinDiskGB
	if floor == 0 {
		floor = 10
	}
	switch {
	case f.DiskFreeGB <= 0:
		c.Status, c.Detail = StatusInfo, "could not be read"
	case f.DiskFreeGB < floor:
		c.Status = StatusFail
		c.Detail = fmt.Sprintf("%.1f GB free on %s, %.0f GB needed for images, builds and backups", f.DiskFreeGB, orDefault(f.DiskPath, "/"), floor)
		c.Fix = "Free space (docker system prune) or move the data directory to a larger disk."
	default:
		c.Status, c.Detail = StatusPass, fmt.Sprintf("%.1f GB free on %s", f.DiskFreeGB, orDefault(f.DiskPath, "/"))
	}
	return c
}

func memoryCheck(f Facts, th Thresholds) Check {
	c := Check{ID: CheckMemory, Name: "Memory"}
	floor := th.MinMemoryMB
	if floor == 0 {
		floor = 1024
	}
	switch {
	case f.MemoryMB <= 0:
		c.Status, c.Detail = StatusInfo, "could not be read"
	case f.MemoryMB < floor:
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("%d MB, %d MB or more recommended", f.MemoryMB, floor)
		c.Fix = "Builds and databases need headroom: add RAM or a swap file."
	default:
		c.Status, c.Detail = StatusPass, fmt.Sprintf("%d MB", f.MemoryMB)
	}
	return c
}

func firewallCheck(f Facts, th Thresholds) Check {
	c := Check{ID: CheckFirewall, Name: "Firewall"}
	switch {
	case !f.Firewall.Known || f.Firewall.Kind == "":
		c.Status, c.Detail = StatusInfo, "no ufw or firewalld found, nothing local blocks ports 80 and 443"
	case !f.Firewall.Active:
		c.Status, c.Detail = StatusPass, f.Firewall.Kind+" is installed but inactive"
	default:
		var missing []string
		for _, p := range []int{th.HTTPPort, th.HTTPSPort} {
			if !containsInt(f.Firewall.Allowed, p) {
				missing = append(missing, strconv.Itoa(p))
			}
		}
		if len(missing) == 0 {
			c.Status, c.Detail = StatusPass, f.Firewall.Kind+" is active and allows 80 and 443"
			break
		}
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("%s is active and does not allow port(s) %s, so certificates cannot be issued and sites are unreachable", f.Firewall.Kind, strings.Join(missing, ", "))
		if f.Firewall.Kind == "ufw" {
			c.Fix = "sudo ufw allow 80/tcp && sudo ufw allow 443/tcp"
		} else {
			c.Fix = "sudo firewall-cmd --permanent --add-service=http --add-service=https && sudo firewall-cmd --reload"
		}
	}
	return c
}

func dnsCheck(f Facts) Check {
	c := Check{ID: CheckDNS, Name: "DNS for " + f.Domain}
	switch {
	case f.DNSError != "":
		c.Status = StatusWarn
		c.Detail = "does not resolve: " + f.DNSError
		c.Fix = "Create an A record for " + f.Domain + " pointing at this server's public IPv4 address, then re-check."
	case len(f.DomainIPs) == 0:
		c.Status, c.Detail = StatusWarn, "resolves to no addresses"
	default:
		ips := append([]string(nil), f.DomainIPs...)
		sort.Strings(ips)
		for _, ip := range ips {
			if containsStr(f.LocalIPs, ip) {
				c.Status, c.Detail = StatusPass, f.Domain+" points at this server ("+ip+")"
				return c
			}
		}
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("%s resolves to %s, which is not an address on this server. Fine behind NAT or a cloud load balancer, otherwise a wrong record", f.Domain, strings.Join(ips, ", "))
		c.Fix = "If this server is reached directly, point the A record at its public IPv4 address."
	}
	return c
}

func recommend(r Report, f Facts, th Thresholds) (next, summary string) {
	prefix := orDefault(th.InstallerPrefix, "sudo")
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return fixOrDetail(c), "Fix the failing check first: " + c.Name + "."
		}
	}
	domainFlag := ""
	if f.Domain != "" {
		domainFlag = " --domain " + f.Domain
	}
	switch r.Mode {
	case ModeBehindProxy:
		who := orDefault(r.Proxy, "another service")
		next = prefix + " sh -s -- --coexist" + domainFlag
		summary = fmt.Sprintf("Ports 80 and 443 are taken by %s. Install behind it: ingress moves to 8088/8443 and the dashboard binds to loopback, your proxy forwards your domain to it.", who)
		if f.Domain != "" {
			next += "\nThen: levelrail-cli proxy --domain " + f.Domain + " --verify"
		}
	default:
		next = prefix + " sh"
		if f.Domain != "" {
			next = prefix + " sh -s --" + domainFlag
		}
		summary = "Ports 80 and 443 are free. Install with its own ports for automatic HTTPS."
	}
	return next, summary
}

func fixOrDetail(c Check) string {
	if c.Fix != "" {
		return c.Fix
	}
	return c.Detail
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func containsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
