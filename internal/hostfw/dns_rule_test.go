package hostfw

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeFW struct {
	tools map[string]bool
	rules map[string]bool // iptables "-i x -p y" present
	calls []string
}

func (f *fakeFW) deps() Deps {
	return Deps{
		LookPath: func(n string) (string, error) {
			if f.tools[n] {
				return "/sbin/" + n, nil
			}
			return "", errors.New("missing")
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			line := name + " " + strings.Join(args, " ")
			f.calls = append(f.calls, line)
			switch {
			case strings.Contains(line, " -S INPUT"):
				var b strings.Builder
				for k := range f.rules {
					b.WriteString("-A INPUT " + k + " -m udp --dport 53 -m comment --comment tagx -j ACCEPT\n")
				}
				b.WriteString("-A INPUT -j REJECT --reject-with icmp-host-prohibited\n")
				return []byte(b.String()), nil
			case strings.Contains(line, " -C INPUT"):
				if f.rules[ruleKey(args)] {
					return nil, nil
				}
				return nil, errors.New("absent")
			case strings.Contains(line, " -I INPUT 1"):
				f.rules[ruleKey(args)] = true
			case strings.Contains(line, " -D INPUT"):
				delete(f.rules, ruleKey(args))
			}
			return nil, nil
		},
	}
}

func ruleKey(args []string) string {
	var i, p string
	for n := range args {
		if args[n] == "-i" {
			i = args[n+1]
		}
		if args[n] == "-p" {
			p = args[n+1]
		}
	}
	return "-i " + i + " -p " + p
}

func TestDNSRulesEnsureIsIdempotentAndRemoves(t *testing.T) {
	f := &fakeFW{tools: map[string]bool{"iptables": true}, rules: map[string]bool{}}
	r := DNSRules{Deps: f.deps(), Tag: "tagx"}
	if err := r.Ensure(context.Background(), []string{"br-abc", "docker0"}); err != nil {
		t.Fatal(err)
	}
	if len(f.rules) != 4 {
		t.Fatalf("want 4 rules, got %v", f.rules)
	}
	f.calls = nil
	if err := r.Ensure(context.Background(), []string{"br-abc", "docker0"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.Contains(c, " -I ") || strings.Contains(c, " -D ") {
			t.Fatalf("second ensure mutated: %s", c)
		}
	}
	if err := r.Ensure(context.Background(), []string{"docker0"}); err != nil {
		t.Fatal(err)
	}
	if len(f.rules) != 2 {
		t.Fatalf("stale bridge rules not dropped: %v", f.rules)
	}
	if err := r.Remove(context.Background()); err != nil || len(f.rules) != 0 {
		t.Fatalf("remove left %v err %v", f.rules, err)
	}
}

func TestDNSRulesRejectsBadInterfaceAndMissingTool(t *testing.T) {
	f := &fakeFW{tools: map[string]bool{"iptables": true}, rules: map[string]bool{}}
	r := DNSRules{Deps: f.deps(), Tag: "tagx"}
	if err := r.Ensure(context.Background(), []string{"br0; rm -rf /"}); !errors.Is(err, ErrBadInterface) {
		t.Fatalf("want ErrBadInterface, got %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("a tool ran for an invalid name: %v", f.calls)
	}
	none := DNSRules{Deps: (&fakeFW{tools: map[string]bool{}}).deps(), Tag: "tagx"}
	if err := none.Ensure(context.Background(), []string{"docker0"}); !errors.Is(err, ErrNoDNSTool) {
		t.Fatalf("want ErrNoDNSTool, got %v", err)
	}
	if err := none.Remove(context.Background()); err != nil {
		t.Fatalf("remove with no tool must be a no-op: %v", err)
	}
}
