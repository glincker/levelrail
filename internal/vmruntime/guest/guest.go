// Package guest holds the files and helpers that run inside the managed VM: the
// egress firewall baseline, the isolation self-check and the provisioning script.
package guest

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed files/*
var embedded embed.FS

// Guest paths the host side (status reads, S2 and S4) relies on.
const (
	StatusPath       = "/var/lib/levelrail/isolation-status.json"
	EgressRulesPath  = "/etc/levelrail/lr-egress.nft"
	CheckScriptPath  = "/usr/local/sbin/lr-isolation-check"
	IsolationUnit    = "lr-isolation.service"
	CheckUnit        = "lr-isolation-check.service"
	dockerDaemonPath = "/etc/docker/daemon.json"
	dockerDropinPath = "/etc/systemd/system/docker.service.d/10-lr-isolation.conf"
	maxUnitBytes     = 64 * 1024
)

// Defaults block: the only place guest network numbers live.
const (
	defaultHostGateway     = "192.168.5.2"
	defaultDNSServer2      = "192.168.5.3"
	defaultDockerPool      = "172.28.0.0/14"
	defaultDockerBridge    = "172.28.0.1/16"
	defaultDockerPoolBase  = "172.29.0.0/16"
	defaultPublishIP       = "127.0.0.1"
	defaultLANProbe        = "10.255.255.254"
	defaultProbeTimeoutSec = 2
)

var unitNameRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]+\.service$`)

// Options tunes the guest baseline. Zero value is invalid, use DefaultOptions.
type Options struct {
	HostGateway     string   // Lima host address as seen from the guest, always dropped.
	DNSServers      []string // Resolvers that may be reached on port 53 despite the drops.
	DockerPool      string   // The only 172.16/12 slice containers may talk to.
	DockerBridge    string   // docker0 address, inside DockerPool.
	DockerPoolBase  string   // User network pool, inside DockerPool.
	PublishIP       string   // Default bind address for published ports.
	LANProbe        string   // Address the self-check expects to be unreachable.
	ProbeTimeoutSec int
	ExpectedShares  int // Read-only host mounts the self-check should find.
}

// DefaultOptions returns the ADR baseline.
func DefaultOptions() Options {
	return Options{
		HostGateway:     defaultHostGateway,
		DNSServers:      []string{defaultHostGateway, defaultDNSServer2},
		DockerPool:      defaultDockerPool,
		DockerBridge:    defaultDockerBridge,
		DockerPoolBase:  defaultDockerPoolBase,
		PublishIP:       defaultPublishIP,
		LANProbe:        defaultLANProbe,
		ProbeTimeoutSec: defaultProbeTimeoutSec,
	}
}

// Sidecar is the control plane slot. S2 and S4 supply the binary and unit; when
// UnitContent is empty the step is skipped.
type Sidecar struct {
	BinaryPath  string // Absolute guest path the unit's ExecStart uses.
	UnitName    string // File name under /etc/systemd/system, ends in .service.
	UnitContent string
}

// File is one guest file to write.
type File struct {
	Path    string
	Mode    string
	Content string
}

// Validate rejects values that could break out of the rendered files.
func (o Options) Validate() error {
	if _, err := netip.ParseAddr(o.HostGateway); err != nil {
		return fmt.Errorf("host gateway: %w", err)
	}
	if len(o.DNSServers) == 0 {
		return fmt.Errorf("at least one DNS server is required")
	}
	for _, d := range o.DNSServers {
		if a, err := netip.ParseAddr(d); err != nil || !a.Is4() {
			return fmt.Errorf("dns server %q is not an IPv4 address", d)
		}
	}
	pool, err := netip.ParsePrefix(o.DockerPool)
	if err != nil || !pool.Addr().Is4() {
		return fmt.Errorf("docker pool %q is not an IPv4 prefix", o.DockerPool)
	}
	private := netip.MustParsePrefix("172.16.0.0/12")
	if !private.Contains(pool.Addr()) || pool.Bits() < private.Bits() {
		return fmt.Errorf("docker pool %q must sit inside %s", o.DockerPool, private)
	}
	for name, v := range map[string]string{"bridge": o.DockerBridge, "pool base": o.DockerPoolBase} {
		p, err := netip.ParsePrefix(v)
		if err != nil || !pool.Contains(p.Addr()) {
			return fmt.Errorf("docker %s %q must sit inside %s", name, v, o.DockerPool)
		}
	}
	if a, err := netip.ParseAddr(o.PublishIP); err != nil || !a.IsLoopback() {
		return fmt.Errorf("publish ip %q must be a loopback address", o.PublishIP)
	}
	if _, err := netip.ParseAddr(o.LANProbe); err != nil {
		return fmt.Errorf("lan probe: %w", err)
	}
	if o.ProbeTimeoutSec < 1 {
		return fmt.Errorf("probe timeout must be at least 1 second")
	}
	if o.ExpectedShares < 0 {
		return fmt.Errorf("expected shares must not be negative")
	}
	return nil
}

// Validate checks the sidecar slot.
func (s Sidecar) Validate() error {
	if s.UnitContent == "" && s.BinaryPath == "" && s.UnitName == "" {
		return nil
	}
	if !unitNameRe.MatchString(s.UnitName) {
		return fmt.Errorf("sidecar unit name %q is invalid", s.UnitName)
	}
	if !strings.HasPrefix(s.BinaryPath, "/") || strings.ContainsAny(s.BinaryPath, " \t\r\n'\"$`\\") {
		return fmt.Errorf("sidecar binary path %q must be absolute with no spaces or quoting", s.BinaryPath)
	}
	if len(s.UnitContent) > maxUnitBytes {
		return fmt.Errorf("sidecar unit is larger than %d bytes", maxUnitBytes)
	}
	return nil
}

type templateData struct {
	Options
	StatusPath string
}

func render(name string, o Options) (string, error) {
	raw, err := embedded.ReadFile("files/" + name)
	if err != nil {
		return "", err
	}
	t, err := template.New(name).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, templateData{Options: o, StatusPath: StatusPath}); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return buf.String(), nil
}

func static(name string) (string, error) {
	raw, err := embedded.ReadFile("files/" + name)
	return string(raw), err
}

// EgressRules renders the nftables baseline.
func EgressRules(o Options) (string, error) {
	if err := o.Validate(); err != nil {
		return "", err
	}
	return render("lr-egress.nft.tmpl", o)
}

// Files returns every guest file in write order, sidecar unit last.
func Files(o Options, sc Sidecar) ([]File, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	var out []File
	add := func(path, mode, tmpl string, isTemplate bool) error {
		var c string
		var err error
		if isTemplate {
			c, err = render(tmpl, o)
		} else {
			c, err = static(tmpl)
		}
		if err != nil {
			return err
		}
		out = append(out, File{Path: path, Mode: mode, Content: c})
		return nil
	}
	steps := []struct {
		path, mode, src string
		tmpl            bool
	}{
		{EgressRulesPath, "0644", "lr-egress.nft.tmpl", true},
		{"/etc/systemd/system/" + IsolationUnit, "0644", "lr-isolation.service", false},
		{dockerDropinPath, "0644", "docker-dropin.conf", false},
		{dockerDaemonPath, "0644", "daemon.json.tmpl", true},
		{CheckScriptPath, "0755", "lr-isolation-check.sh.tmpl", true},
		{"/etc/systemd/system/" + CheckUnit, "0644", "lr-isolation-check.service", false},
	}
	for _, s := range steps {
		if err := add(s.path, s.mode, s.src, s.tmpl); err != nil {
			return nil, err
		}
	}
	if sc.UnitContent != "" {
		out = append(out, File{Path: "/etc/systemd/system/" + sc.UnitName, Mode: "0644", Content: sc.UnitContent})
	}
	return out, nil
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// ProvisionScript returns one root shell script: files first, then the firewall,
// then Docker (so Docker can only start behind the baseline), then the sidecar.
func ProvisionScript(o Options, sc Sidecar) (string, error) {
	files, err := Files(o, sc)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("#!/bin/bash\nset -eux -o pipefail\n")
	for i, f := range files {
		delim := fmt.Sprintf("LR_EOF_%d", i)
		if strings.Contains(f.Content, delim) {
			return "", fmt.Errorf("content of %s contains the heredoc delimiter", f.Path)
		}
		fmt.Fprintf(&b, "install -d -m 0755 \"$(dirname %s)\"\n", f.Path)
		fmt.Fprintf(&b, "cat > %s <<'%s'\n%s%s\n", f.Path, delim, ensureTrailingNewline(f.Content), delim)
		fmt.Fprintf(&b, "chmod %s %s\n", f.Mode, f.Path)
	}
	b.WriteString("export DEBIAN_FRONTEND=noninteractive\n")
	b.WriteString("apt-get update -y\napt-get install -y --no-install-recommends nftables iproute2\n")
	b.WriteString("systemctl daemon-reload\nsystemctl enable --now " + IsolationUnit + "\n")
	b.WriteString("apt-get install -y --no-install-recommends docker.io\n")
	b.WriteString("systemctl enable --now docker.service\n")
	if sc.UnitContent != "" {
		fmt.Fprintf(&b, "if [ -x %s ]; then systemctl enable --now %s; fi\n", sc.BinaryPath, sc.UnitName)
	}
	b.WriteString("systemctl enable " + CheckUnit + "\nsystemctl restart " + CheckUnit + " || true\n")
	return b.String(), nil
}

type cloudConfig struct {
	Packages   []string    `yaml:"packages"`
	WriteFiles []cloudFile `yaml:"write_files"`
	RunCmd     [][]string  `yaml:"runcmd"`
}

type cloudFile struct {
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions"`
	Owner       string `yaml:"owner"`
	Content     string `yaml:"content"`
}

// UserData renders the same baseline as cloud-init user-data, for images that
// boot without Lima (marketplace images, the WSL slice).
func UserData(o Options, sc Sidecar) (string, error) {
	files, err := Files(o, sc)
	if err != nil {
		return "", err
	}
	cc := cloudConfig{Packages: []string{"nftables", "iproute2"}}
	for _, f := range files {
		cc.WriteFiles = append(cc.WriteFiles, cloudFile{Path: f.Path, Permissions: f.Mode, Owner: "root:root", Content: f.Content})
	}
	cc.RunCmd = [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", IsolationUnit},
		{"apt-get", "install", "-y", "--no-install-recommends", "docker.io"},
		{"systemctl", "enable", "--now", "docker.service"},
		{"systemctl", "enable", "--now", CheckUnit},
	}
	if sc.UnitContent != "" {
		cc.RunCmd = append(cc.RunCmd, []string{"systemctl", "enable", "--now", sc.UnitName})
	}
	raw, err := yaml.Marshal(cc)
	if err != nil {
		return "", err
	}
	return "#cloud-config\n" + string(raw), nil
}

// Check is one line of the guest self-check result.
type Check struct {
	Name   string `json:"name"`
	Result string `json:"result"`
	Detail string `json:"detail"`
}

// Status is the parsed self-check file.
type Status struct {
	Result    string  `json:"result"`
	CheckedAt string  `json:"checked_at"`
	Checks    []Check `json:"checks"`
}

// Passed is true only when every check passed.
func (s Status) Passed() bool {
	if s.Result != "pass" || len(s.Checks) == 0 {
		return false
	}
	for _, c := range s.Checks {
		if c.Result != "pass" {
			return false
		}
	}
	return true
}

// ParseStatus parses the file read from StatusPath. Unknown shapes fail closed.
func ParseStatus(raw []byte) (Status, error) {
	var s Status
	if err := json.Unmarshal(raw, &s); err != nil {
		return Status{}, fmt.Errorf("isolation status: %w", err)
	}
	if s.Result != "pass" && s.Result != "fail" {
		return Status{}, fmt.Errorf("isolation status: unknown result %q", s.Result)
	}
	return s, nil
}
