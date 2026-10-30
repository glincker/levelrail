package exposure

import (
	"context"
	"strings"
	"testing"
)

func TestClassifyUDPMatchesOnlyUDPRules(t *testing.T) {
	tcpOnly := chainOf("-A DOCKER-USER -p tcp -m conntrack --ctorigdstport 53 --ctdir ORIGINAL -j DROP")
	udpDrop := chainOf("-A DOCKER-USER ! -s 10.0.0.0/8 -p udp -m udp --dport 53 -j DROP")
	managed := chainOf(
		"-A DOCKER-USER -s 10.0.0.0/8 -p udp -m conntrack --ctorigdstport 51820 --ctdir ORIGINAL -m comment --comment acme:exposure:udp/51820 -j RETURN",
		"-A DOCKER-USER -p udp -m conntrack --ctorigdstport 51820 --ctdir ORIGINAL -m comment --comment acme:exposure:udp/51820 -j DROP")
	tests := []struct {
		name  string
		chain Chain
		port  int
		proto string
		want  Class
	}{
		{"tcp rule leaves udp exposed", tcpOnly, 53, "udp", ClassExposed},
		{"tcp rule restricts tcp", tcpOnly, 53, "tcp", ClassRestricted},
		{"udp container port drop", udpDrop, 53, "udp", ClassRestricted},
		{"udp rule leaves tcp exposed", udpDrop, 53, "tcp", ClassExposed},
		{"managed udp allow list", managed, 51820, "udp", ClassRestricted},
		{"managed udp is not tcp", managed, 51820, "tcp", ClassExposed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify([]string{"0.0.0.0"}, tc.port, tc.port, tc.proto, tc.chain, testPrefix); got.Class != tc.want {
				t.Fatalf("class = %s, want %s (%s)", got.Class, tc.want, got.Reason)
			}
		})
	}
}

func TestApplyUDPRestrictionKeepsTCPApart(t *testing.T) {
	f := &fakeChain{rules: []string{"-j RETURN"}}
	m := newTestManager(f)
	ctx := context.Background()
	for _, proto := range []string{"udp", "tcp"} {
		if _, err := m.Apply(ctx, Restriction{Port: 5353, Protocol: proto, Allow: []string{"10.0.0.0/8"}}); err != nil {
			t.Fatalf("apply %s: %v", proto, err)
		}
	}
	udpRules := 0
	for _, r := range f.rules {
		if strings.Contains(r, "acme:exposure:udp/5353") {
			udpRules++
			if !strings.Contains(r, "-p udp") || !strings.Contains(r, "--ctorigdstport 5353") {
				t.Fatalf("udp rule = %q, want -p udp and --ctorigdstport", r)
			}
		}
	}
	if udpRules != 2 {
		t.Fatalf("rules = %v, want an allow and a drop tagged udp/5353", f.rules)
	}
	if err := m.Remove(ctx, 5353, "udp"); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.rules {
		if strings.Contains(r, "udp/5353") {
			t.Fatalf("udp rule left after remove: %v", f.rules)
		}
	}
	if !strings.Contains(strings.Join(f.rules, "\n"), "acme:exposure:tcp/5353") {
		t.Fatalf("removing udp took the tcp rules too: %v", f.rules)
	}
}
