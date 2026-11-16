package hostfw

import (
	"context"
	"fmt"
	"strings"
)

// Finding codes for a host firewall that would still drop DNS from a bridge.
const (
	CodeManagedFirewalld = "dns_firewall_managed_firewalld"
	CodeManagedUFW       = "dns_firewall_managed_ufw"
	CodeRuleShadowed     = "dns_firewall_rule_shadowed"
	CodeNftNoTable       = "dns_firewall_nft_no_table"
)

// DNSFinding is one detected problem plus the command an operator can run. We only ever print it.
type DNSFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Command string `json:"command"`
}

// Diagnose reports firewall states that make a present accept rule useless. It never changes the host.
func (r DNSRules) Diagnose(ctx context.Context, ifaces []string) []DNSFinding {
	if r.check(ifaces) != nil || len(ifaces) == 0 {
		return nil
	}
	var out []DNSFinding
	if f, ok := r.managedFirewalld(ctx, ifaces); ok {
		out = append(out, f)
	}
	if f, ok := r.managedUFW(ctx, ifaces); ok {
		out = append(out, f)
	}
	switch r.Backend() {
	case BackendIptables:
		out = append(out, r.shadowed(ctx, ifaces)...)
	case BackendNftables:
		if f, ok := r.nftNoTable(ctx); ok {
			out = append(out, f)
		}
	}
	return out
}

func (r DNSRules) managedFirewalld(ctx context.Context, ifaces []string) (DNSFinding, bool) {
	if _, err := r.Deps.LookPath("firewall-cmd"); err != nil {
		return DNSFinding{}, false
	}
	raw, err := r.Deps.Run(ctx, "firewall-cmd", "--state")
	if err != nil || strings.TrimSpace(string(raw)) != "running" {
		return DNSFinding{}, false
	}
	cmds := make([]string, 0, len(ifaces)+1)
	for _, n := range ifaces {
		cmds = append(cmds, "sudo firewall-cmd --permanent --zone=trusted --add-interface="+n)
	}
	cmds = append(cmds, "sudo firewall-cmd --reload")
	return DNSFinding{
		Code:    CodeManagedFirewalld,
		Message: "firewalld is running and manages this host, so its zones can drop DNS from the bridges even with our INPUT accept",
		Command: strings.Join(cmds, " && "),
	}, true
}

func (r DNSRules) managedUFW(ctx context.Context, ifaces []string) (DNSFinding, bool) {
	if _, err := r.Deps.LookPath("ufw"); err != nil {
		return DNSFinding{}, false
	}
	raw, err := r.Deps.Run(ctx, "ufw", "status")
	if err != nil || !strings.Contains(string(raw), "Status: active") {
		return DNSFinding{}, false
	}
	cmds := make([]string, 0, len(ifaces))
	for _, n := range ifaces {
		cmds = append(cmds, "sudo ufw allow in on "+n+" to any port 53")
	}
	return DNSFinding{
		Code:    CodeManagedUFW,
		Message: "ufw is active on this host, so its rules can drop DNS from the bridges even with our INPUT accept",
		Command: strings.Join(cmds, " && "),
	}, true
}

// shadowed flags a tagged accept that sits behind a REJECT or DROP which would catch the same traffic.
func (r DNSRules) shadowed(ctx context.Context, ifaces []string) []DNSFinding {
	raw, err := r.Deps.Run(ctx, "iptables", "-w", "-S", "INPUT")
	if err != nil {
		return nil
	}
	want := map[string]bool{}
	for _, n := range ifaces {
		want[n] = true
	}
	var out []DNSFinding
	reported := map[string]bool{}
	var terminals [][]string
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[0] != "-A" {
			continue
		}
		if k, ok := r.taggedKey(f); ok && want[k.iface] {
			if reported[k.iface] || !anyCatches(terminals, k) {
				continue
			}
			reported[k.iface] = true
			out = append(out, DNSFinding{
				Code:    CodeRuleShadowed,
				Message: fmt.Sprintf("a REJECT or DROP rule sits before our accept for %s in INPUT, so DNS from that bridge is still refused", k.iface),
				Command: r.reinsertCommand(k.iface),
			})
			continue
		}
		if j := flagValue(f, "-j"); j == "REJECT" || j == "DROP" {
			terminals = append(terminals, f)
		}
	}
	return out
}

func (r DNSRules) taggedKey(f []string) (dnsRuleKey, bool) {
	iface, proto := flagValue(f, "-i"), flagValue(f, "-p")
	tag := strings.Trim(flagValue(f, "--comment"), `"`)
	if tag != r.Tag || !ifaceRe.MatchString(iface) || (proto != "udp" && proto != "tcp") {
		return dnsRuleKey{}, false
	}
	return dnsRuleKey{iface, proto}, true
}

// anyCatches is true when an earlier terminal rule is not narrowed away from this interface, protocol and port 53.
func anyCatches(terminals [][]string, k dnsRuleKey) bool {
	for _, t := range terminals {
		if i := flagValue(t, "-i"); i != "" && i != k.iface {
			continue
		}
		if p := flagValue(t, "-p"); p != "" && p != "all" && p != k.proto {
			continue
		}
		if d := flagValue(t, "--dport"); d != "" && d != "53" {
			continue
		}
		if flagValue(t, "--dports") != "" || flagValue(t, "-s") != "" {
			continue
		}
		return true
	}
	return false
}

func (r DNSRules) reinsertCommand(iface string) string {
	cmds := make([]string, 0, 4)
	for _, proto := range []string{"udp", "tcp"} {
		args := strings.Join(r.iptArgs("-D", dnsRuleKey{iface, proto}), " ")
		cmds = append(cmds, "sudo iptables "+args)
	}
	for _, proto := range []string{"udp", "tcp"} {
		args := strings.Join(r.iptArgs("-I", dnsRuleKey{iface, proto}), " ")
		cmds = append(cmds, "sudo iptables "+args)
	}
	return strings.Join(cmds, " && ")
}

func (r DNSRules) nftNoTable(ctx context.Context) (DNSFinding, bool) {
	raw, err := r.Deps.Run(ctx, "nft", "list", "table", "inet", "filter")
	if err == nil || unreadable(raw, err) {
		return DNSFinding{}, false
	}
	return DNSFinding{
		Code:    CodeNftNoTable,
		Message: "nft is the rule tool here but there is no inet filter table, so the DNS accept cannot be inserted",
		Command: "sudo nft add table inet filter && sudo nft add chain inet filter input '{ type filter hook input priority 0 ; }'",
	}, true
}

func flagValue(f []string, name string) string {
	for i := 0; i+1 < len(f); i++ {
		if f[i] == name {
			return f[i+1]
		}
	}
	return ""
}
