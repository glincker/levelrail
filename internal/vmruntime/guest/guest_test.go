package guest

import (
	"net/netip"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEgressRulesBlockedRangesAndOrder(t *testing.T) {
	rules, err := EgressRules(DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ip daddr 192.168.5.2 drop", "10.0.0.0/8", "192.168.0.0/16", "169.254.0.0/16",
		"100.64.0.0/10", "127.0.0.0/8", "ip daddr 172.16.0.0/12 ip daddr != 172.28.0.0/14 drop",
		"fc00::/7", "fe80::/10", "hook forward priority -10", "hook output priority -10", "hook input priority -10",
		"ip saddr 172.28.0.0/14 drop", "ip6 daddr { fc00::/7 } drop",
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("rules lack %q", want)
		}
	}
	// DNS must be accepted before the gateway drop, or name resolution breaks.
	dns := strings.Index(rules, "ip daddr 192.168.5.2 udp dport 53 accept")
	drop := strings.Index(rules, "ip daddr 192.168.5.2 drop")
	est := strings.Index(rules, "ct state established,related accept")
	if est < 0 || est >= dns || dns >= drop {
		t.Errorf("rule order wrong: est=%d dns=%d drop=%d", est, dns, drop)
	}
}

func TestFilesAndUnits(t *testing.T) {
	files, err := Files(DefaultOptions(), Sidecar{})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]File{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	unit := byPath["/etc/systemd/system/"+IsolationUnit].Content
	if !strings.Contains(unit, "Before=network.target docker.service") {
		t.Error("isolation unit must precede Docker")
	}
	if !strings.Contains(byPath[dockerDropinPath].Content, "Requires=lr-isolation.service") {
		t.Error("docker must require the isolation unit")
	}
	if !strings.Contains(byPath[dockerDaemonPath].Content, `"ip": "127.0.0.1"`) {
		t.Error("docker must publish to loopback by default")
	}
	chk := byPath[CheckScriptPath].Content
	for _, want := range []string{StatusPath, "host_unreachable", "lan_unreachable", "no_wildcard_publish"} {
		if !strings.Contains(chk, want) {
			t.Errorf("check script lacks %q", want)
		}
	}
}

func TestUserDataParsesAndMatchesFiles(t *testing.T) {
	ud, err := UserData(DefaultOptions(), Sidecar{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ud, "#cloud-config\n") {
		t.Fatal("missing cloud-config header")
	}
	var cc map[string]any
	if err := yaml.Unmarshal([]byte(ud), &cc); err != nil {
		t.Fatal(err)
	}
	if n := len(cc["write_files"].([]any)); n != 6 {
		t.Errorf("write_files = %d", n)
	}
	if wildcard.MatchString(ud) {
		t.Error("no wildcard binds")
	}
}

func TestOptionsAndSidecarRejects(t *testing.T) {
	bad := map[string]func(*Options){
		"gateway":      func(o *Options) { o.HostGateway = "nope" },
		"no dns":       func(o *Options) { o.DNSServers = nil },
		"pool":         func(o *Options) { o.DockerPool = "192.168.0.0/16" },
		"bridge":       func(o *Options) { o.DockerBridge = "172.20.0.1/16" },
		"publish ip":   func(o *Options) { o.PublishIP = "0.0.0.0" },
		"zero timeout": func(o *Options) { o.ProbeTimeoutSec = 0 },
		"gw zone":      func(o *Options) { o.HostGateway = "fe80::1%\"; touch /x #" },
		"gw v6":        func(o *Options) { o.HostGateway = "fe80::1" },
		"gw newline":   func(o *Options) { o.HostGateway = "192.168.5.2\nflush ruleset" },
		"probe zone":   func(o *Options) { o.LANProbe = "fe80::1%eth0" },
		"probe range":  func(o *Options) { o.LANProbe = "10.255.255.254" },
		"publish zone": func(o *Options) { o.PublishIP = "::1%lo" },
		"publish v6":   func(o *Options) { o.PublishIP = "::1" },
		"bridge v6":    func(o *Options) { o.DockerBridge = "fe80::1/64" },
		"pool base nl": func(o *Options) { o.DockerPoolBase = "172.29.0.0/16\nx" },
	}
	for name, mut := range bad {
		o := DefaultOptions()
		mut(&o)
		if _, err := EgressRules(o); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	for _, sc := range []Sidecar{
		{BinaryPath: "relative", UnitName: "a.service", UnitContent: "x"},
		{BinaryPath: "/a b", UnitName: "a.service", UnitContent: "x"},
		{BinaryPath: "/bin/a", UnitName: "../a.service", UnitContent: "x"},
	} {
		if err := sc.Validate(); err == nil {
			t.Errorf("sidecar %+v should be rejected", sc)
		}
	}
}

func TestParseStatus(t *testing.T) {
	ok := `{"result":"pass","boot_id":"b1","checked_at":"x","checks":[{"name":"a","result":"pass","detail":"ok"}]}`
	s, err := ParseStatus([]byte(ok), "b1\n")
	if err != nil || !s.Passed() {
		t.Fatalf("pass status: %v %+v", err, s)
	}
	if s, _ := ParseStatus([]byte(`{"result":"pass","boot_id":"b1","checked_at":"x","checks":[{"name":"a","result":"fail","detail":""}]}`), "b1"); s.Passed() {
		t.Error("a failed check must not pass")
	}
	if _, err := ParseStatus([]byte(`{"result":"maybe"}`), "b1"); err == nil {
		t.Error("unknown result must fail closed")
	}
	if _, err := ParseStatus([]byte(ok), "other"); err == nil {
		t.Error("a status from another boot must be rejected")
	}
	if _, err := ParseStatus([]byte(`{"result":"pass","boot_id":"b1","checks":[]}`), "b1"); err == nil {
		t.Error("missing checked_at must be rejected")
	}
	if _, err := ParseStatus([]byte(`garbage`), "b1"); err == nil {
		t.Error("garbage must fail")
	}
}

// Matches a bare 0.0.0.0 but not 10.0.0.0/8.
var wildcard = regexp.MustCompile(`(^|[^0-9.])0\.0\.0\.0`)

func TestAppNetworkPoolInsideAllowedRange(t *testing.T) {
	o := DefaultOptions()
	pool := netip.MustParsePrefix(o.AppNetworkPool)
	if !netip.MustParsePrefix(o.DockerPool).Contains(pool.Addr()) {
		t.Fatalf("app pool %s is outside the allowed docker pool %s", pool, o.DockerPool)
	}
	dropped := []string{"10.0.0.0/8", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "127.0.0.0/8", "224.0.0.0/4", o.HostGateway + "/32", o.DockerBridge, o.DockerPoolBase}
	for _, d := range dropped {
		if pool.Overlaps(netip.MustParsePrefix(d).Masked()) {
			t.Errorf("app pool %s overlaps %s", pool, d)
		}
	}
	for _, bad := range []string{"10.88.0.0/14", "172.20.0.0/15", "172.28.0.0/15", "172.29.0.0/16", "172.30.0.5/15"} {
		o := DefaultOptions()
		o.AppNetworkPool = bad
		if o.Validate() == nil {
			t.Errorf("pool %s should be rejected", bad)
		}
	}
	rules, err := EgressRules(o)
	if err != nil || !strings.Contains(rules, "ip daddr != "+o.DockerPool+" drop") {
		t.Errorf("firewall does not allow the docker pool: %v", err)
	}
	files, err := Files(o, Sidecar{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == CheckScriptPath && !strings.Contains(f.Content, "app_pool_allowed") {
			t.Error("isolation check must verify the app pool")
		}
	}
}
