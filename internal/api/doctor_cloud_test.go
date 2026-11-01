package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const oracleInputRules = `-P INPUT ACCEPT
-A INPUT -m state --state RELATED,ESTABLISHED -j ACCEPT
-A INPUT -p icmp -j ACCEPT
-A INPUT -i lo -j ACCEPT
-A INPUT -p tcp -m state --state NEW -m tcp --dport 22 -j ACCEPT
-A INPUT -j REJECT --reject-with icmp-host-prohibited
`

func fakeCloudDeps(dmi map[string]string, tools map[string]bool, cmds map[string]string) cloudDeps {
	return cloudDeps{
		getenv: func(string) string { return "" },
		readFile: func(p string) ([]byte, error) {
			if v, ok := dmi[strings.TrimPrefix(p, "/sys/class/dmi/id/")]; ok {
				return []byte(v + "\n"), nil
			}
			return nil, errors.New("absent")
		},
		lookPath: func(n string) (string, error) {
			if tools[n] {
				return "/bin/" + n, nil
			}
			return "", errors.New("missing")
		},
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if v, ok := cmds[name+" "+strings.Join(args, " ")]; ok {
				return []byte(v), nil
			}
			return nil, errors.New("no fixture")
		},
		hostArch: "arm64",
	}
}

func cloudByCode(rs []doctorCheckResource, code string) *doctorCheckResource {
	for i := range rs {
		if rs[i].Code == code {
			return &rs[i]
		}
	}
	return nil
}

func TestDetectCloudProvider(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dmi  map[string]string
		want string
	}{
		{"oracle", map[string]string{"chassis_asset_tag": "OracleCloud.com"}, "oracle"},
		{"aws", map[string]string{"sys_vendor": "Amazon EC2"}, "aws"},
		{"gcp", map[string]string{"product_name": "Google Compute Engine"}, "gcp"},
		{"azure", map[string]string{"chassis_asset_tag": "7783-7084-3265-9085-8269-3286-77"}, "azure"},
		{"hetzner", map[string]string{"sys_vendor": "Hetzner"}, "hetzner"},
		{"bare metal", map[string]string{"sys_vendor": "Dell Inc."}, ""},
		{"no dmi", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, _ := detectCloudProvider(context.Background(), fakeCloudDeps(tc.dmi, nil, nil))
			got := ""
			if p != nil {
				got = p.ID
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestDetectCloudProvider_MetadataIsOptIn(t *testing.T) {
	t.Parallel()
	calls := 0
	d := fakeCloudDeps(nil, nil, nil)
	d.metadata = func(context.Context, string) ([]byte, error) {
		calls++
		return []byte(`{"shape":"VM.Standard.A1.Flex"}`), nil
	}
	if p, _ := detectCloudProvider(context.Background(), d); p != nil || calls != 0 {
		t.Fatalf("metadata used while off: p=%v calls=%d", p, calls)
	}
	d.getenv = func(k string) string {
		if k == envDoctorCloudMetadata {
			return "on"
		}
		return ""
	}
	p, shape := detectCloudProvider(context.Background(), d)
	if p == nil || p.ID != "oracle" || shape != "VM.Standard.A1.Flex" {
		t.Fatalf("got %v %q", p, shape)
	}
}

func TestCloudAnalyzeInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		rules  string
		pos    int
		accept bool
	}{
		{"reject last", oracleInputRules, 5, false},
		{"accept appended after reject", oracleInputRules + "-A INPUT -p udp -m udp --dport 19132 -j ACCEPT\n", 5, true},
		{"no reject", "-A INPUT -p tcp --dport 80 -j ACCEPT\n", 0, false},
		{"conditional reject ignored", "-A INPUT -p tcp --dport 25 -j REJECT\n-A INPUT -p tcp --dport 80 -j ACCEPT\n", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pos, acc := cloudAnalyzeInput(tc.rules)
			if pos != tc.pos || acc != tc.accept {
				t.Fatalf("got %d,%v want %d,%v", pos, acc, tc.pos, tc.accept)
			}
		})
	}
}

func TestRunCloudChecks_OracleIptables(t *testing.T) {
	t.Parallel()
	d := fakeCloudDeps(
		map[string]string{"chassis_asset_tag": "OracleCloud.com"},
		map[string]bool{"iptables": true, "netfilter-persistent": true},
		map[string]string{"iptables -S INPUT": oracleInputRules + "-A INPUT -p udp -m udp --dport 19132 -j ACCEPT\n"},
	)
	d.ports = func(context.Context) ([]cloudPort, error) {
		return []cloudPort{{Port: 19132, Proto: "udp", Image: "itzg/minecraft-bedrock-server"}, {Port: 19132, Proto: "udp"}, {Port: 99999, Proto: "tcp"}, {Port: 25565, Proto: "tcp; rm"}}, nil
	}
	d.imageArch = func(context.Context, string) (string, error) { return "amd64", nil }
	rs := runCloudChecks(context.Background(), d)

	for _, code := range []string{"cloud_provider", "cloud_firewall_layers", "host_firewall_iptables", "published_port_reachability", "image_arch", "idle_reclaim_risk"} {
		if cloudByCode(rs, code) == nil {
			t.Errorf("missing %s", code)
		}
	}
	fw := cloudByCode(rs, "host_firewall_iptables")
	if fw.Status != doctorStatusWarn {
		t.Errorf("status %s", fw.Status)
	}
	want := "sudo iptables -I INPUT 5 -p udp --dport 19132 -j ACCEPT\nsudo netfilter-persistent save"
	if fw.Fix != want {
		t.Errorf("fix %q", fw.Fix)
	}
	if strings.Contains(fw.Fix, "99999") || strings.Contains(fw.Fix, "rm") {
		t.Error("unsafe port leaked into commands")
	}
	if !strings.Contains(cloudByCode(rs, "cloud_firewall_layers").Message, "VCN security list") {
		t.Error("oracle wording missing")
	}
	idle := cloudByCode(rs, "idle_reclaim_risk")
	if !strings.Contains(idle.Message, oracleFreeTierDocs) || !strings.Contains(idle.Message, "can change") || !strings.Contains(idle.Message, "20 percent over 7 days") {
		t.Errorf("idle wording: %s", idle.Message)
	}
	if cloudByCode(rs, "image_arch").Status != doctorStatusWarn {
		t.Error("arch mismatch should warn")
	}
}

func TestRunCloudChecks_IdleThresholdsFromEnv(t *testing.T) {
	t.Parallel()
	d := fakeCloudDeps(map[string]string{"chassis_asset_tag": "OracleCloud.com"}, nil, nil)
	d.getenv = func(k string) string {
		switch k {
		case envDoctorOracleIdlePercent:
			return "15"
		case envDoctorOracleIdleDays:
			return "10"
		}
		return ""
	}
	idle := cloudByCode(runCloudChecks(context.Background(), d), "idle_reclaim_risk")
	if idle == nil || !strings.Contains(idle.Message, "15 percent over 10 days") {
		t.Fatalf("got %+v", idle)
	}
}

func TestRunCloudChecks_NotApplicableIsSilent(t *testing.T) {
	t.Parallel()
	cases := map[string]cloudDeps{
		"bare host no tools":            fakeCloudDeps(nil, nil, nil),
		"unreadable iptables off cloud": fakeCloudDeps(nil, map[string]bool{"iptables": true}, nil),
		"ufw active":                    fakeCloudDeps(nil, map[string]bool{"iptables": true, "ufw": true}, map[string]string{"ufw status": "Status: active"}),
	}
	for name, d := range cases {
		rs := runCloudChecks(context.Background(), d)
		if name == "ufw active" {
			if len(rs) != 1 || rs[0].Status != doctorStatusOK {
				t.Errorf("%s: %+v", name, rs)
			}
			continue
		}
		if len(rs) != 0 {
			t.Errorf("%s: expected nothing, got %+v", name, rs)
		}
	}
}

func TestRunCloudChecks_SkipAndPaidShape(t *testing.T) {
	t.Parallel()
	d := fakeCloudDeps(map[string]string{"chassis_asset_tag": "OracleCloud.com"}, nil, nil)
	d.getenv = func(k string) string {
		if k == envDoctorCloudChecks {
			return "off"
		}
		return ""
	}
	if rs := runCloudChecks(context.Background(), d); len(rs) != 0 {
		t.Fatalf("skip ignored: %+v", rs)
	}
	if _, ok := idleReclaimResult(fakeCloudDeps(nil, nil, nil), &cloudProviderInfo{ID: "oracle"}, "VM.Standard.E4.Flex"); ok {
		t.Fatal("paid shape should not get the idle note")
	}
}

func TestPublishedPortsResult_Bindings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		binds     [][]string
		wantOpen  string
		wantNone  string
		wantNoRes bool
	}{
		{name: "loopback only", binds: [][]string{{"127.0.0.1"}}, wantNone: "25565/tcp", wantOpen: "nothing needs opening"},
		{name: "wildcard", binds: [][]string{{"0.0.0.0"}}, wantOpen: "Published ports: 25565/tcp"},
		{name: "ipv6 wildcard", binds: [][]string{{"::"}}, wantOpen: "Published ports: 25565/tcp"},
		{name: "ipv6 loopback", binds: [][]string{{"::1"}}, wantOpen: "nothing needs opening"},
		{name: "public interface ip", binds: [][]string{{"10.0.0.5"}}, wantOpen: "Published ports: 25565/tcp"},
		{name: "loopback and wildcard on one port", binds: [][]string{{"127.0.0.1", "0.0.0.0"}}, wantOpen: "Published ports: 25565/tcp"},
		{name: "no bind info", binds: [][]string{nil}, wantOpen: "Published ports: 25565/tcp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var ports []cloudPort
			for _, b := range tc.binds {
				ports = append(ports, cloudPort{Port: 25565, Proto: "tcp", Loopback: cloudBindsLoopback(b)})
			}
			res, ok := publishedPortsResult(cloudDedupPorts(ports), nil)
			if !ok || !strings.Contains(res.Message, tc.wantOpen) {
				t.Fatalf("ok=%v message %q want %q", ok, res.Message, tc.wantOpen)
			}
		})
	}
}

func TestPublishedPortsResult_MixedListsOnlyReachable(t *testing.T) {
	t.Parallel()
	ports := cloudDedupPorts([]cloudPort{
		{Port: 32895, Proto: "tcp", Loopback: true},
		{Port: 25565, Proto: "tcp", Loopback: false},
		{Port: 32895, Proto: "tcp", Loopback: true},
	})
	res, _ := publishedPortsResult(ports, nil)
	if !strings.Contains(res.Message, "Published ports: 25565/tcp.") || !strings.Contains(res.Message, "Loopback-only ports need no firewall change: 32895/tcp") {
		t.Fatalf("message %q", res.Message)
	}
	if cmds := cloudOpenCommands(ports, 5, "persist"); strings.Contains(cmds, "32895") {
		t.Fatalf("loopback port in open commands: %s", cmds)
	}
}

func TestImageArchResult_WordingCoversEmulation(t *testing.T) {
	t.Parallel()
	d := cloudDeps{hostArch: "arm64", imageArch: func(context.Context, string) (string, error) { return "amd64", nil }}
	res, ok := imageArchResult(context.Background(), d, []cloudPort{{Port: 1, Proto: "tcp", Image: "x/y:1"}})
	if !ok || !strings.Contains(res.Message, "pull or start fails with PlatformMismatch") || !strings.Contains(res.Message, "emulation") {
		t.Fatalf("message %q", res.Message)
	}
}

func TestDesiredCloudPorts_Scenarios(t *testing.T) {
	t.Parallel()
	stream := store.AppStream{ServiceName: "mc", HostPort: 25565, Protocol: "tcp"}
	direct := store.DesiredService{Name: "mc", BindAddress: "private", PublishedPorts: []store.PublishedPort{{HostPort: 25565, ContainerPort: 25565, Bind: "public"}}}
	loop := store.DesiredService{Name: "mc", BindAddress: "private", PublishedPorts: []store.PublishedPort{{HostPort: 25565, ContainerPort: 25565}}}
	udp := store.DesiredService{Name: "vpn", BindAddress: "public", PublishedPorts: []store.PublishedPort{{HostPort: 51820, ContainerPort: 51820, Protocol: "udp"}}}
	cases := []struct {
		name     string
		svcs     []store.DesiredService
		streams  []store.AppStream
		wantOpen string
		wantPriv string
		wantNone bool
	}{
		{name: "stream only", svcs: []store.DesiredService{loop}, streams: []store.AppStream{stream}, wantOpen: "Published ports: 25565/tcp."},
		{name: "direct only", svcs: []store.DesiredService{direct}, wantOpen: "Published ports: 25565/tcp."},
		{name: "stream plus loopback template publish", svcs: []store.DesiredService{{Name: "mc", PublishedPorts: []store.PublishedPort{{HostPort: 32895, ContainerPort: 25565}}}}, streams: []store.AppStream{stream}, wantOpen: "Published ports: 25565/tcp.", wantPriv: "32895/tcp"},
		{name: "mixed", svcs: []store.DesiredService{direct, udp}, streams: []store.AppStream{{ServiceName: "x", HostPort: 8080, Protocol: "tcp"}}, wantOpen: "8080/tcp, 25565/tcp, 51820/udp"},
		{name: "none", wantNone: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ports := cloudDedupPorts(desiredCloudPorts(tc.svcs, tc.streams))
			res, ok := publishedPortsResult(ports, nil)
			if tc.wantNone {
				if ok {
					t.Fatalf("unexpected result %q", res.Message)
				}
				return
			}
			if !ok || !strings.Contains(res.Message, tc.wantOpen) || !strings.Contains(res.Message, tc.wantPriv) {
				t.Fatalf("ok=%v message %q want %q private %q", ok, res.Message, tc.wantOpen, tc.wantPriv)
			}
			cmds := cloudOpenCommands(ports, 5, "persist")
			if tc.wantOpen != "" && !strings.Contains(cmds, "iptables -I INPUT 5 -p ") {
				t.Fatalf("commands %q", cmds)
			}
		})
	}
}

func TestClearStaleSuspended(t *testing.T) {
	t.Parallel()
	stale := []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: "Suspended"}}
	if got := clearStaleSuspended(store.DesiredService{Suspended: false}, stale); got[0].Reason != "Starting" {
		t.Fatalf("started app still shows %q", got[0].Reason)
	}
	if got := clearStaleSuspended(store.DesiredService{Suspended: true}, stale); got[0].Reason != "Suspended" {
		t.Fatalf("stopped app lost Suspended: %q", got[0].Reason)
	}
	if stale[0].Reason != "Suspended" {
		t.Fatal("input mutated")
	}
}

func TestRunCloudChecks_ImageArchCoversFailedDeploy(t *testing.T) {
	t.Parallel()
	// Mirrors the store: per-service list, newest first.
	attempts := map[string][]store.DeployAttempt{
		"broken": {{Image: "acme/amd-only:1", Status: store.DeployAttemptStatusFailed}, {Image: "acme/old:1", Status: store.DeployAttemptStatusSucceeded}},
		"fixed":  {{Image: "acme/ok:2", Status: store.DeployAttemptStatusSucceeded}, {Image: "acme/amd-only:0", Status: store.DeployAttemptStatusFailed}},
	}
	list := func(_ context.Context, n string) ([]store.DeployAttempt, error) {
		if n == "none" {
			return nil, errors.New("list failed")
		}
		return attempts[n], nil
	}
	d := fakeCloudDeps(nil, nil, nil)
	d.failedImages = func(ctx context.Context) []string {
		return latestFailedImages(ctx, []string{"broken", "fixed", "none"}, list)
	}
	d.imageArch = func(context.Context, string) (string, error) { return "amd64", nil }
	r := cloudByCode(runCloudChecks(context.Background(), d), "image_arch")
	if r == nil || !strings.Contains(r.Message, "acme/amd-only:1 is amd64") || strings.Contains(r.Message, ":0") {
		t.Fatalf("got %+v", r)
	}
}
