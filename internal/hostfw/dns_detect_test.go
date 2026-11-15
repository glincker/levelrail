package hostfw

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func capturedDeps(tools map[string]bool, out map[string]string) Deps {
	return Deps{
		LookPath: func(n string) (string, error) {
			if tools[n] {
				return "/sbin/" + n, nil
			}
			return "", errors.New("missing")
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			line := name + " " + strings.Join(args, " ")
			for k, v := range out {
				if strings.HasPrefix(line, k) {
					if strings.HasPrefix(v, "ERR:") {
						return []byte(v[4:]), errors.New("exit 1")
					}
					return []byte(v), nil
				}
			}
			return nil, errors.New("unexpected " + line)
		},
	}
}

func codes(fs []DNSFinding) string {
	var c []string
	for _, f := range fs {
		c = append(c, f.Code)
	}
	return strings.Join(c, ",")
}

const oracleChain = "-P INPUT ACCEPT\n" +
	"-A INPUT -m state --state RELATED,ESTABLISHED -j ACCEPT\n" +
	"-A INPUT -p icmp -j ACCEPT\n"

func TestDiagnoseOracleChainRuleBeforeRejectIsClean(t *testing.T) {
	ipt := oracleChain +
		"-A INPUT -i docker0 -p udp -m udp --dport 53 -m comment --comment tagx -j ACCEPT\n" +
		"-A INPUT -i docker0 -p tcp -m tcp --dport 53 -m comment --comment tagx -j ACCEPT\n" +
		"-A INPUT -p tcp -m tcp --dport 22 -j ACCEPT\n-A INPUT -j REJECT --reject-with icmp-host-prohibited\n"
	r := DNSRules{Tag: "tagx", Deps: capturedDeps(map[string]bool{"iptables": true}, map[string]string{"iptables -w -S INPUT": ipt})}
	if got := r.Diagnose(context.Background(), []string{"docker0"}); len(got) != 0 {
		t.Fatalf("clean chain flagged: %v", got)
	}
}

func TestDiagnoseShadowedByReject(t *testing.T) {
	ipt := oracleChain + "-A INPUT -j REJECT --reject-with icmp-host-prohibited\n" +
		"-A INPUT -i docker0 -p udp -m udp --dport 53 -m comment --comment tagx -j ACCEPT\n"
	r := DNSRules{Tag: "tagx", Deps: capturedDeps(map[string]bool{"iptables": true}, map[string]string{"iptables -w -S INPUT": ipt})}
	got := r.Diagnose(context.Background(), []string{"docker0"})
	if codes(got) != CodeRuleShadowed || !strings.Contains(got[0].Command, "-I INPUT 1 -i docker0") {
		t.Fatalf("want shadowed with reinsert command, got %+v", got)
	}
}

func TestDiagnoseUnrelatedDropDoesNotShadow(t *testing.T) {
	ipt := "-A INPUT -p tcp -m tcp --dport 23 -j DROP\n-A INPUT -i eth0 -j DROP\n" +
		"-A INPUT -i docker0 -p udp -m udp --dport 53 -m comment --comment tagx -j ACCEPT\n"
	r := DNSRules{Tag: "tagx", Deps: capturedDeps(map[string]bool{"iptables": true}, map[string]string{"iptables -w -S INPUT": ipt})}
	if got := r.Diagnose(context.Background(), []string{"docker0"}); len(got) != 0 {
		t.Fatalf("narrow drops flagged: %v", got)
	}
}

func TestDiagnoseUFWAndFirewalldPrintCommandsOnly(t *testing.T) {
	tools := map[string]bool{"iptables": true, "ufw": true, "firewall-cmd": true}
	out := map[string]string{
		"firewall-cmd --state": "running\n",
		"ufw status":           "Status: active\n",
		"iptables -w -S INPUT": "-P INPUT DROP\n",
	}
	d := capturedDeps(tools, out)
	var ran []string
	inner := d.Run
	d.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return inner(ctx, name, args...)
	}
	got := DNSRules{Tag: "tagx", Deps: d}.Diagnose(context.Background(), []string{"docker0", "br-ab"})
	if codes(got) != CodeManagedFirewalld+","+CodeManagedUFW {
		t.Fatalf("codes %s", codes(got))
	}
	if !strings.Contains(got[0].Command, "--zone=trusted --add-interface=br-ab") ||
		!strings.Contains(got[1].Command, "sudo ufw allow in on docker0 to any port 53") {
		t.Fatalf("commands %+v", got)
	}
	for _, c := range ran {
		if strings.Contains(c, "allow") || strings.Contains(c, "--add") || strings.Contains(c, "-I ") {
			t.Fatalf("mutating command executed: %s", c)
		}
	}
}

func TestDiagnoseInactiveManagersAreQuiet(t *testing.T) {
	tools := map[string]bool{"ufw": true, "firewall-cmd": true}
	out := map[string]string{"firewall-cmd --state": "ERR:not running", "ufw status": "Status: inactive\n"}
	if got := (DNSRules{Tag: "tagx", Deps: capturedDeps(tools, out)}).Diagnose(context.Background(), []string{"docker0"}); len(got) != 0 {
		t.Fatalf("inactive flagged: %v", got)
	}
}

func TestDiagnoseNftWithoutInetFilter(t *testing.T) {
	tools := map[string]bool{"nft": true}
	r := DNSRules{Tag: "tagx", Deps: capturedDeps(tools, map[string]string{"nft list table inet filter": "ERR:Error: No such file or directory"})}
	got := r.Diagnose(context.Background(), []string{"docker0"})
	if codes(got) != CodeNftNoTable || !strings.Contains(got[0].Command, "nft add table inet filter") {
		t.Fatalf("got %+v", got)
	}
	ok := DNSRules{Tag: "tagx", Deps: capturedDeps(tools, map[string]string{"nft list table inet filter": "table inet filter {}"})}
	if got := ok.Diagnose(context.Background(), []string{"docker0"}); len(got) != 0 {
		t.Fatalf("existing table flagged: %v", got)
	}
}
