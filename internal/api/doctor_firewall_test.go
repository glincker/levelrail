package api

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDoctorCheckFirewall_NotInstalled(t *testing.T) {
	lookPath := func(string) (string, error) { return "", errors.New("not found") }
	run := func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("run should not be called when ufw isn't on PATH")
		return nil, nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusUnknown {
		t.Errorf("Status = %q, want %q", got.Status, doctorStatusUnknown)
	}
}

func TestDoctorCheckFirewall_ActiveDenyIncoming(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Status: active\nLogging: on (low)\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n"), nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusOK {
		t.Errorf("Status = %q, want %q, message = %q", got.Status, doctorStatusOK, got.Message)
	}
}

func TestDoctorCheckFirewall_ActiveAllowIncoming(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Status: active\nDefault: allow (incoming), allow (outgoing), disabled (routed)\n"), nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusWarn {
		t.Errorf("Status = %q, want %q (allow incoming is a real warning), message = %q", got.Status, doctorStatusWarn, got.Message)
	}
}

func TestDoctorCheckFirewall_InstalledButInactive(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Status: inactive\n"), nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusWarn {
		t.Errorf("Status = %q, want %q, message = %q", got.Status, doctorStatusWarn, got.Message)
	}
}

func TestDoctorCheckFirewall_UFWFixAllowsSSHBeforeEnabling(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	for name, status := range map[string]string{
		"inactive":       "Status: inactive\n",
		"allow incoming": "Status: active\nDefault: allow (incoming), allow (outgoing), disabled (routed)\n",
	} {
		t.Run(name, func(t *testing.T) {
			run := func(context.Context, string, ...string) ([]byte, error) { return []byte(status), nil }
			got := doctorCheckFirewall(context.Background(), lookPath, run)
			ssh := strings.Index(got.Fix, "22/tcp")
			lockout := strings.Index(got.Fix, "ufw enable")
			if lockout < 0 {
				lockout = strings.Index(got.Fix, "default deny")
			}
			if ssh < 0 || lockout < 0 || ssh > lockout {
				t.Errorf("Fix = %q, want SSH allowed before the step that can lock the operator out", got.Fix)
			}
			if !strings.Contains(got.Fix, "443/udp") {
				t.Errorf("Fix = %q, want 443/udp (HTTP/3) allowed", got.Fix)
			}
		})
	}
}

func TestDoctorCheckFirewall_CommandFails(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return nil, &exec.ExitError{}
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusUnknown {
		t.Errorf("Status = %q, want %q", got.Status, doctorStatusUnknown)
	}
}

// lookPathOnly returns a lookPath fake that reports found only for
// tool, not found for everything else, so a firewalld/nftables/iptables
// test never accidentally falls through to a different backend.
func lookPathOnly(tool string) func(string) (string, error) {
	return func(name string) (string, error) {
		if name == tool {
			return "/usr/bin/" + tool, nil
		}
		return "", errors.New("not found")
	}
}

func TestDoctorCheckFirewall_Firewalld(t *testing.T) {
	tests := []struct {
		name       string
		run        firewallCommandRunner
		wantStatus string
	}{
		{
			name: "running with http/https open",
			run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				switch {
				case len(args) == 1 && args[0] == "--state":
					return []byte("running\n"), nil
				default:
					return []byte("yes\n"), nil
				}
			},
			wantStatus: doctorStatusOK,
		},
		{
			name:       "not running",
			run:        func(context.Context, string, ...string) ([]byte, error) { return []byte("not running\n"), nil },
			wantStatus: doctorStatusWarn,
		},
		{
			name: "running but https closed",
			run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				switch {
				case len(args) == 1 && args[0] == "--state":
					return []byte("running\n"), nil
				case len(args) == 1 && args[0] == "--query-service=https":
					return []byte("no\n"), nil
				default:
					return []byte("yes\n"), nil
				}
			},
			wantStatus: doctorStatusWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := doctorCheckFirewall(context.Background(), lookPathOnly("firewall-cmd"), tt.run)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q, message = %q", got.Status, tt.wantStatus, got.Message)
			}
			if tt.wantStatus == doctorStatusWarn && got.Fix == "" {
				t.Error("Fix = \"\", want a concrete firewall-cmd command")
			}
		})
	}
}

func TestDoctorCheckFirewall_Nftables(t *testing.T) {
	tests := []struct {
		name       string
		ruleset    string
		wantStatus string
	}{
		{name: "input policy drop with web ports accepted", ruleset: nftFilterFixture, wantStatus: doctorStatusOK},
		{name: "Docker NAT only", ruleset: nftDockerNATFixture, wantStatus: doctorStatusWarn},
		{name: "empty ruleset", ruleset: "", wantStatus: doctorStatusWarn},
		{name: "filtering input chain without web ports", ruleset: strings.Replace(nftFilterFixture, "80, 443", "22", 1), wantStatus: doctorStatusWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := func(context.Context, string, ...string) ([]byte, error) { return []byte(tt.ruleset), nil }
			got := doctorCheckFirewall(context.Background(), lookPathOnly("nft"), run)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q, message = %q", got.Status, tt.wantStatus, got.Message)
			}
		})
	}
}

func TestDoctorCheckFirewall_Iptables(t *testing.T) {
	tests := []struct {
		name       string
		rules      string
		wantStatus string
	}{
		{name: "policy drop with explicit web rules", rules: "-P INPUT DROP\n-A INPUT -p tcp --dport 80 -j ACCEPT\n-A INPUT -p tcp --dport 443 -j ACCEPT\n", wantStatus: doctorStatusOK},
		{name: "ACCEPT-all with web rules", rules: "-P INPUT ACCEPT\n-A INPUT -p tcp --dport 80 -j ACCEPT\n-A INPUT -p tcp --dport 443 -j ACCEPT\n", wantStatus: doctorStatusWarn},
		{name: "empty ufw chains, policy ACCEPT", rules: "-P INPUT ACCEPT\n-N ufw-before-input\n-A INPUT -j ufw-before-input\n", wantStatus: doctorStatusWarn},
		{name: "no matching rules", rules: "-P INPUT ACCEPT\n", wantStatus: doctorStatusWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := func(context.Context, string, ...string) ([]byte, error) { return []byte(tt.rules), nil }
			got := doctorCheckFirewall(context.Background(), lookPathOnly("iptables"), run)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q, message = %q", got.Status, tt.wantStatus, got.Message)
			}
			if tt.wantStatus == doctorStatusWarn && got.Fix == "" {
				t.Error("Fix = \"\", want a concrete iptables command")
			}
		})
	}
}

func TestDoctorCheckFirewall_PrefersUFWOverOtherBackends(t *testing.T) {
	lookPath := func(name string) (string, error) { return "/usr/bin/" + name, nil } // every tool "found"
	run := func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name != "ufw" {
			t.Fatalf("run called with %q, want only ufw to be probed when all tools are present", name)
		}
		return []byte("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n"), nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Name != "Firewall (ufw)" {
		t.Errorf("Name = %q, want ufw to win when every backend is present", got.Name)
	}
}

func TestDoctorCheckFirewallCtx_RealDependencies(t *testing.T) {
	// Smoke test only: proves the real exec.LookPath/exec.CommandContext
	// wiring doesn't panic or hang, whatever ufw's actual state on the
	// machine running this test happens to be (CI runners typically
	// don't have ufw installed at all, which is itself a valid,
	// non-failing outcome per doctorCheckFirewall's own doc comment).
	got := doctorCheckFirewallCtx(context.Background())
	if got.Code != "firewall" {
		t.Errorf("Code = %q, want %q", got.Code, "firewall")
	}
	if got.Status == doctorStatusFail {
		t.Errorf("Status = %q, want never fail (informational check only)", got.Status)
	}
}

const nftFilterFixture = `table inet filter {
	chain input {
		type filter hook input priority filter; policy drop;
		tcp dport { 80, 443 } accept
	}
}
`

// Docker's own NAT rules mention 80 and 443 but filter nothing inbound.
const nftDockerNATFixture = `table ip nat {
	chain DOCKER {
		tcp dport 443 dnat to 172.17.0.2:443
		tcp dport 80 dnat to 172.17.0.2:80
	}
}
table ip filter {
	chain INPUT {
		type filter hook input priority filter; policy accept;
	}
	chain FORWARD {
		type filter hook forward priority filter; policy drop;
	}
}
`

func TestDoctorCheckFirewall_InputPolicyMatrix(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		output     string
		wantStatus string
	}{
		{name: "ufw active deny", tool: "ufw", output: "Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n", wantStatus: doctorStatusOK},
		{name: "nft with filter", tool: "nft", output: nftFilterFixture, wantStatus: doctorStatusOK},
		{name: "iptables ACCEPT-all", tool: "iptables", output: "-P INPUT ACCEPT\n-P FORWARD DROP\n", wantStatus: doctorStatusWarn},
		{name: "Docker NAT only", tool: "nft", output: nftDockerNATFixture, wantStatus: doctorStatusWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := func(context.Context, string, ...string) ([]byte, error) { return []byte(tt.output), nil }
			got := doctorCheckFirewall(context.Background(), lookPathOnly(tt.tool), run)
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q, message = %q", got.Status, tt.wantStatus, got.Message)
			}
			if tt.wantStatus == doctorStatusWarn && (got.Fix == "" || !strings.Contains(got.Message, "INPUT accepts everything")) {
				t.Errorf("warn needs next step and inbound message, got %+v", got)
			}
		})
	}
}
