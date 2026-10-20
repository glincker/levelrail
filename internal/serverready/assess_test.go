package serverready

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func healthy() Facts {
	return Facts{
		OS: "linux", Arch: "amd64", Systemd: true,
		DockerPresent: true, DockerVersion: "27.3.1",
		DiskFreeGB: 80, DiskPath: "/var/lib/levelrail-data", MemoryMB: 4096,
		Firewall: Firewall{Known: true, Kind: "ufw", Active: true, Allowed: []int{80, 443}},
	}
}

func status(r Report, id string) string {
	for _, c := range r.Checks {
		if c.ID == id {
			return c.Status
		}
	}
	return "missing"
}

func TestAssessPortSituations(t *testing.T) {
	tests := []struct {
		name        string
		listeners   []Listener
		self        []string
		wantMode    string
		wantProxy   string
		wantPort80  string
		wantNextHas string
	}{
		{name: "both ports free", wantMode: ModeOwnPorts, wantPort80: StatusPass, wantNextHas: "sh"},
		{
			name:      "traefik container holds both",
			listeners: []Listener{{Addr: "0.0.0.0", Port: 80, Process: "docker-proxy", Container: "coolify-proxy", Image: "traefik:v3.1"}, {Addr: "0.0.0.0", Port: 443, Process: "docker-proxy", Container: "coolify-proxy", Image: "traefik:v3.1"}},
			wantMode:  ModeBehindProxy, wantProxy: ProxyTraefik, wantPort80: StatusWarn, wantNextHas: "--coexist",
		},
		{
			name:      "host nginx holds both",
			listeners: []Listener{{Addr: "0.0.0.0", Port: 80, Process: "nginx", PID: 812}, {Addr: "[::]", Port: 443, Process: "nginx", PID: 812}},
			wantMode:  ModeBehindProxy, wantProxy: ProxyNginx, wantPort80: StatusWarn, wantNextHas: "--coexist",
		},
		{
			name:      "caddy on 443 only",
			listeners: []Listener{{Addr: "*", Port: 443, Process: "caddy", PID: 77}},
			wantMode:  ModeBehindProxy, wantProxy: ProxyCaddy, wantPort80: StatusPass, wantNextHas: "--coexist",
		},
		{
			name:      "unidentified process on 80",
			listeners: []Listener{{Addr: "0.0.0.0", Port: 80}},
			wantMode:  ModeBehindProxy, wantProxy: "", wantPort80: StatusWarn, wantNextHas: "--coexist",
		},
		{
			name:      "apache on 80",
			listeners: []Listener{{Addr: "0.0.0.0", Port: 80, Process: "apache2", PID: 5}},
			wantMode:  ModeBehindProxy, wantProxy: ProxyApache, wantPort80: StatusWarn, wantNextHas: "--coexist",
		},
		{
			name:      "ports held by this installation are not a conflict",
			listeners: []Listener{{Addr: "0.0.0.0", Port: 80, Process: "levelrail"}, {Addr: "0.0.0.0", Port: 443, Process: "levelrail"}},
			self:      []string{"levelrail"},
			wantMode:  ModeOwnPorts, wantPort80: StatusPass, wantNextHas: "sh",
		},
		{
			name:      "unrelated listener on another port",
			listeners: []Listener{{Addr: "0.0.0.0", Port: 8080, Process: "node"}},
			wantMode:  ModeOwnPorts, wantPort80: StatusPass, wantNextHas: "sh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := healthy()
			f.Listeners, f.SelfProcesses = tt.listeners, tt.self
			r := Assess(f, Thresholds{})
			if r.Mode != tt.wantMode || r.Proxy != tt.wantProxy {
				t.Fatalf("mode=%s proxy=%q, want %s %q", r.Mode, r.Proxy, tt.wantMode, tt.wantProxy)
			}
			if got := status(r, CheckPort80); got != tt.wantPort80 {
				t.Fatalf("port 80 status = %s, want %s", got, tt.wantPort80)
			}
			if !strings.Contains(r.NextStep, tt.wantNextHas) {
				t.Fatalf("next step %q lacks %q", r.NextStep, tt.wantNextHas)
			}
			if r.Blocked {
				t.Fatalf("port contention must not block an install: %+v", r.Checks)
			}
		})
	}
}

func TestAssessHostChecks(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(f *Facts)
		check   string
		want    string
		blocked bool
	}{
		{"not linux", func(f *Facts) { f.OS = "darwin" }, CheckOS, StatusFail, true},
		{"no systemd", func(f *Facts) { f.Systemd = false }, CheckOS, StatusFail, true},
		{"arm 32", func(f *Facts) { f.Arch = "arm" }, CheckOS, StatusFail, true},
		{"docker absent is installable", func(f *Facts) { f.DockerPresent = false }, CheckDocker, StatusWarn, false},
		{"docker too old", func(f *Facts) { f.DockerVersion = "20.10.7" }, CheckDocker, StatusFail, true},
		{"docker not answering", func(f *Facts) { f.DockerError = "permission denied" }, CheckDocker, StatusFail, true},
		{"rootless docker", func(f *Facts) { f.DockerRootless = true }, CheckDockerRootles, StatusWarn, false},
		{"docker tcp exposed", func(f *Facts) { f.Listeners = []Listener{{Addr: "0.0.0.0", Port: 2375, Process: "dockerd"}} }, CheckDockerSocket, StatusFail, true},
		{"docker tcp on loopback is fine", func(f *Facts) { f.Listeners = []Listener{{Addr: "127.0.0.1", Port: 2375}} }, CheckDockerSocket, StatusPass, false},
		{"socket world writable", func(f *Facts) { f.DockerSocketWorldWritable = true }, CheckDockerSocket, StatusWarn, false},
		{"disk full", func(f *Facts) { f.DiskFreeGB = 2.5 }, CheckDisk, StatusFail, true},
		{"disk unknown", func(f *Facts) { f.DiskFreeGB = 0 }, CheckDisk, StatusInfo, false},
		{"little memory", func(f *Facts) { f.MemoryMB = 512 }, CheckMemory, StatusWarn, false},
		{"firewall blocks https", func(f *Facts) { f.Firewall.Allowed = []int{80} }, CheckFirewall, StatusWarn, false},
		{"firewall inactive", func(f *Facts) { f.Firewall.Active = false }, CheckFirewall, StatusPass, false},
		{"no firewall", func(f *Facts) { f.Firewall = Firewall{} }, CheckFirewall, StatusInfo, false},
		{"dns not resolving", func(f *Facts) { f.Domain, f.DNSError = "app.example.com", "no such host" }, CheckDNS, StatusWarn, false},
		{"dns points here", func(f *Facts) {
			f.Domain, f.DomainIPs, f.LocalIPs = "app.example.com", []string{"203.0.113.9"}, []string{"10.0.0.5", "203.0.113.9"}
		}, CheckDNS, StatusPass, false},
		{"dns points elsewhere", func(f *Facts) {
			f.Domain, f.DomainIPs, f.LocalIPs = "app.example.com", []string{"198.51.100.1"}, []string{"10.0.0.5"}
		}, CheckDNS, StatusWarn, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := healthy()
			tt.mutate(&f)
			r := Assess(f, Thresholds{})
			if got := status(r, tt.check); got != tt.want {
				t.Fatalf("%s = %s, want %s: %+v", tt.check, got, tt.want, r.Checks)
			}
			if r.Blocked != tt.blocked {
				t.Fatalf("blocked = %v, want %v", r.Blocked, tt.blocked)
			}
		})
	}
}

func TestAssessBlockedNextStepIsTheFix(t *testing.T) {
	f := healthy()
	f.DockerVersion = "19.03.1"
	r := Assess(f, Thresholds{})
	if !r.Blocked || !strings.Contains(r.NextStep, "docs.docker.com") {
		t.Fatalf("next step %q", r.NextStep)
	}
}

func TestAssessDomainFlowsIntoNextStep(t *testing.T) {
	f := healthy()
	f.Domain = "console.example.com"
	f.Listeners = []Listener{{Addr: "0.0.0.0", Port: 443, Process: "traefik"}}
	r := Assess(f, Thresholds{})
	for _, want := range []string{"--coexist --domain console.example.com", "levelrail-cli proxy --domain console.example.com --verify"} {
		if !strings.Contains(r.NextStep, want) {
			t.Fatalf("next step %q lacks %q", r.NextStep, want)
		}
	}
}

func TestParseSS(t *testing.T) {
	out := `LISTEN 0      511          0.0.0.0:80         0.0.0.0:*    users:(("nginx",pid=812,fd=6),("nginx",pid=813,fd=6))
LISTEN 0      4096               *:443              *:*    users:(("docker-proxy",pid=999,fd=4))
LISTEN 0      128             [::]:22             [::]:*
LISTEN 0      4096       127.0.0.1:2019       0.0.0.0:*    users:(("levelrail",pid=7,fd=9))
garbage line
`
	got := ParseSS(out)
	want := []Listener{
		{Addr: "0.0.0.0", Port: 80, Process: "nginx", PID: 812},
		{Addr: "*", Port: 443, Process: "docker-proxy", PID: 999},
		{Addr: "::", Port: 22},
		{Addr: "127.0.0.1", Port: 2019, Process: "levelrail", PID: 7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseUFW(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want Firewall
	}{
		{"inactive", "Status: inactive\n", Firewall{Kind: "ufw", Known: true}},
		{"web ports", "Status: active\n\nTo                         Action      From\n--                         ------      ----\n22/tcp                     ALLOW       Anywhere\n80/tcp                     ALLOW       Anywhere\n443                        ALLOW       Anywhere\n", Firewall{Kind: "ufw", Known: true, Active: true, Allowed: []int{80, 443}}},
		{"app profile", "Status: active\n\nNginx Full                 ALLOW       Anywhere\n", Firewall{Kind: "ufw", Known: true, Active: true, Allowed: []int{80, 443}}},
		{"range and list", "Status: active\n\n8000:9000/tcp               ALLOW       Anywhere\n80,443/tcp                 ALLOW       Anywhere\n", Firewall{Kind: "ufw", Known: true, Active: true, Allowed: []int{80, 443}}},
		{"ssh only", "Status: active\n\n22/tcp                     ALLOW       Anywhere\n", Firewall{Kind: "ufw", Known: true, Active: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseUFW(tt.out); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestParseFirewalld(t *testing.T) {
	got := ParseFirewalld("8080/tcp 443/tcp", "ssh http")
	if !reflect.DeepEqual(got.Allowed, []int{8080, 443, 80}) || !got.Active || got.Kind != "firewalld" {
		t.Fatalf("got %+v", got)
	}
}

func TestCollectSurvivesFailingProbes(t *testing.T) {
	boom := errors.New("no ss")
	p := Probes{
		SS:         func(context.Context) (string, error) { return "", boom },
		UFW:        func(context.Context) (string, error) { return "", boom },
		DockerInfo: func(context.Context) (DockerInfo, error) { return DockerInfo{}, errors.New("cannot connect") },
		SocketMode: func(string) (os.FileMode, error) { return 0o666, nil },
		MemoryMB:   func() (int, error) { return 0, boom },
		LookupHost: func(context.Context, string) ([]string, error) { return nil, errors.New("nxdomain") },
		HasSystemd: func() bool { return true },
	}
	f := Collect(context.Background(), p, Request{Domain: " App.Example.com ", DataDir: "/data"})
	if f.Domain != "app.example.com" || f.DNSError != "nxdomain" || f.DockerError == "" || !f.DockerSocketWorldWritable || !f.Systemd {
		t.Fatalf("facts = %+v", f)
	}
	r := Assess(f, Thresholds{})
	if len(r.Checks) == 0 || r.NextStep == "" {
		t.Fatalf("report = %+v", r)
	}
}

func TestMergePublishedAttachesContainer(t *testing.T) {
	ls := []Listener{{Addr: "0.0.0.0", Port: 80, Process: "docker-proxy", PID: 5}}
	got := mergePublished(ls, []Listener{{Port: 80, Container: "proxy", Image: "traefik:v3"}, {Port: 443, Container: "proxy", Image: "traefik:v3"}})
	if got[0].Container != "proxy" || len(got) != 2 || got[1].Port != 443 {
		t.Fatalf("got %+v", got)
	}
}
