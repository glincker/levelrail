package exposure

import (
	"slices"
	"strings"
	"testing"
)

const driftPrefix = "lr-"

func exposedFinding(name string, owner OwnerKind, port int, kind ImageKind) Finding {
	return Finding{Container: name, Owner: Owner{Kind: owner}, Protocol: "tcp", HostPort: port, ContainerPort: port, ImageKind: kind, Class: ClassExposed, Severity: SeverityHigh}
}

func driftKinds(fs []DriftFinding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, string(f.Kind)+":"+restrictionID(f.Protocol, f.Port))
	}
	return out
}

func TestCheckDrift(t *testing.T) {
	// vps2: hand-made DROP rules for 5432 and 8108 with no tag.
	vps2 := ParseChain(`-N DOCKER-USER
-A DOCKER-USER -p tcp -m tcp --dport 5432 -j DROP
-A DOCKER-USER -p tcp -m tcp --dport 8108 -j DROP
-A DOCKER-USER -j RETURN
`)
	tagged := ParseChain(`-A DOCKER-USER -p tcp -m conntrack --ctorigdstport 6379 --ctdir ORIGINAL -s 10.0.0.1/32 -m comment --comment "lr-exposure:tcp/6379" -j RETURN
-A DOCKER-USER -p tcp -m conntrack --ctorigdstport 6379 --ctdir ORIGINAL -m comment --comment "lr-exposure:tcp/6379" -j DROP
`)
	cases := []struct {
		name string
		in   DriftInput
		want []string
	}{
		{"clean", DriftInput{Chain: ParseChain("-N DOCKER-USER\n"), Prefix: driftPrefix}, nil},
		{
			"vps2 hand-made rules",
			DriftInput{Chain: vps2, Prefix: driftPrefix},
			[]string{"restriction_undeclared:tcp/5432", "restriction_undeclared:tcp/8108"},
		},
		{
			"declared restriction missing",
			DriftInput{Chain: ParseChain("-N DOCKER-USER\n"), Declared: []Restriction{{Port: 6379, Protocol: "tcp", Allow: []string{"10.0.0.1/32"}}}, Prefix: driftPrefix},
			[]string{"restriction_missing:tcp/6379"},
		},
		{
			"declared restriction in place",
			DriftInput{Chain: tagged, Declared: []Restriction{{Port: 6379, Allow: []string{"10.0.0.1/32"}}}, Prefix: driftPrefix},
			nil,
		},
		{
			"tagged rules without a record",
			DriftInput{Chain: tagged, Prefix: driftPrefix},
			[]string{"restriction_undeclared:tcp/6379"},
		},
		{
			"unreadable chain skips rule checks",
			DriftInput{Chain: Chain{Reason: "no iptables"}, Declared: []Restriction{{Port: 6379}}, Prefix: driftPrefix},
			nil,
		},
		{
			"unmanaged exposed datastore",
			DriftInput{Chain: ParseChain(""), Findings: []Finding{exposedFinding("pg", OwnerUnmanaged, 5432, KindDatastore)}, Prefix: driftPrefix},
			[]string{"unmanaged_public_port:tcp/5432"},
		},
		{
			"unmanaged web port is not drift",
			DriftInput{Chain: ParseChain(""), Findings: []Finding{exposedFinding("site", OwnerUnmanaged, 8080, KindWeb)}, Prefix: driftPrefix},
			nil,
		},
		{
			"managed exposed port is declared",
			DriftInput{Chain: ParseChain(""), Findings: []Finding{exposedFinding("db", OwnerDatabase, 5432, KindDatastore)}, Prefix: driftPrefix},
			nil,
		},
		{
			"undeclared listener 5390",
			DriftInput{
				Chain: ParseChain(""), Prefix: driftPrefix, Known: []int{22},
				Findings: []Finding{{Protocol: "tcp", HostPort: 443, Class: ClassExposed, Owner: Owner{Kind: OwnerApp}}},
				Listeners: []Listener{
					{Protocol: "tcp", Address: "0.0.0.0", Port: 22}, {Protocol: "tcp", Address: "0.0.0.0", Port: 443}, {Protocol: "tcp", Address: "127.0.0.1", Port: 9000}, {Protocol: "tcp", Address: "[::]", Port: 5390},
				},
			},
			[]string{"listener_undeclared:tcp/5390"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := driftKinds(CheckDrift(tc.in))
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCheckDriftSeverities(t *testing.T) {
	got := CheckDrift(DriftInput{Chain: ParseChain("-A DOCKER-USER -p tcp -m tcp --dport 5432 -j DROP\n"), Prefix: driftPrefix})
	if len(got) != 1 || got[0].Severity != SeverityInfo || len(got[0].Rules) != 1 {
		t.Fatalf("hand-made rule must be info with the rule attached: %+v", got)
	}
}

func TestParseListeners(t *testing.T) {
	out := `tcp LISTEN 0 4096 0.0.0.0:22 0.0.0.0:*
tcp LISTEN 0 4096 [::]:443 [::]:*
udp UNCONN 0 0 127.0.0.53%lo:53 0.0.0.0:*
garbage
tcp LISTEN 0 4096 *:8080 *:*
`
	got := ParseListeners(out)
	if len(got) != 4 || got[1].Port != 443 || got[1].Address != "[::]" || got[3].Address != "*" {
		t.Fatalf("unexpected: %+v", got)
	}
	if publicAddr(got[2].Address) || !publicAddr(got[1].Address) || !publicAddr(got[3].Address) {
		t.Fatal("publicAddr misclassified")
	}
}

func TestListenerDriftNamesProcess(t *testing.T) {
	out := `tcp LISTEN 0 4096 0.0.0.0:5390 0.0.0.0:* users:(("wg-helper",pid=811,fd=7))
udp UNCONN 0 0 *:51820 *:* users:(("wireguard-go",pid=902,fd=3),("x",pid=1,fd=4))
tcp LISTEN 0 4096 0.0.0.0:9999 0.0.0.0:*
`
	ls := ParseListeners(out)
	if len(ls) != 3 || ls[0].Process != "wg-helper" || ls[0].PID != 811 || ls[1].Process != "wireguard-go" || ls[2].Process != "" {
		t.Fatalf("process parse: %+v", ls)
	}
	got := CheckDrift(DriftInput{Listeners: ls, Prefix: driftPrefix})
	if len(got) != 3 {
		t.Fatalf("want 3 findings, got %+v", got)
	}
	byPort := map[int]DriftFinding{}
	for _, f := range got {
		byPort[f.Port] = f
	}
	if m := byPort[5390].Message; !strings.Contains(m, "process wg-helper (pid 811)") {
		t.Errorf("named message = %q", m)
	}
	if m := byPort[9999].Message; !strings.HasPrefix(m, "something listens") {
		t.Errorf("unknown owner must fall back, got %q", m)
	}
}
