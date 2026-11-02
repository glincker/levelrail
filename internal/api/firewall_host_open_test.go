package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/hostfw"
)

func TestHostFirewallStatus_OpenCommandsPerBackend(t *testing.T) {
	const nftDrop = "table ip filter {\n\tchain INPUT {\n\t\ttype filter hook input priority filter; policy drop;\n\t}\n}\n"
	tests := []struct {
		name string
		tool string
		outs map[string]string
		want string
	}{
		{"nftables", "nft", map[string]string{"nft list ruleset": nftDrop}, "nft add rule inet filter input tcp dport 22 accept"},
		{"iptables", "iptables", map[string]string{"iptables -S": "-P INPUT DROP\n"}, "iptables -I INPUT -p tcp --dport 22 -j ACCEPT"},
		{"firewalld", "firewall-cmd", map[string]string{"firewall-cmd --state": "running\n"}, "firewall-cmd --permanent --add-port=22/tcp && firewall-cmd --reload"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := hostFirewallBackendStatus(t, tc.tool, tc.outs)
			found := false
			for _, c := range res.OpenCommands {
				if strings.HasPrefix(c.Command, "ufw") {
					t.Errorf("ufw command %q for a %s host", c.Command, tc.name)
				}
				found = found || c.Command == tc.want
			}
			if !found {
				t.Errorf("open commands = %+v, want %q", res.OpenCommands, tc.want)
			}
		})
	}
}

func TestHostFirewallStatus_NoOpenCommandsWhenNotBlocked(t *testing.T) {
	res := hostFirewallBackendStatus(t, "iptables", map[string]string{"iptables -S": "-P INPUT ACCEPT\n"})
	if len(res.OpenCommands) != 0 {
		t.Errorf("open commands = %+v, want none when nothing filters inbound", res.OpenCommands)
	}
}

func TestDoctorFirewallFixesShareOpenFix(t *testing.T) {
	look := func(n string) (string, error) {
		if n == "iptables" {
			return n, nil
		}
		return "", errors.New("not found")
	}
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("-P INPUT DROP\n"), nil
	}
	got := doctorCheckFirewall(t.Context(), look, run)
	if !strings.Contains(got.Fix, hostfw.OpenFix(hostfw.BackendIptables, hostfw.WebPorts)) {
		t.Errorf("doctor fix %q does not use the shared helper", got.Fix)
	}
}
