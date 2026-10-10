package exposure

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const testPrefix = "acme:"

func chainOf(lines ...string) Chain {
	return ParseChain(strings.Join(lines, "\n"))
}

func TestClassifyTable(t *testing.T) {
	unreadable := Chain{Reason: "no iptables"}
	dropAll := chainOf("-N DOCKER-USER",
		"-A DOCKER-USER -s 10.0.0.5/32 -p tcp -m conntrack --ctorigdstport 5432 --ctdir ORIGINAL -j RETURN",
		"-A DOCKER-USER -p tcp -m conntrack --ctorigdstport 5432 --ctdir ORIGINAL -m comment --comment acme:exposure:tcp/5432 -j DROP",
		"-A DOCKER-USER -j RETURN")
	containerPortDrop := chainOf("-A DOCKER-USER ! -s 10.0.0.0/8 -p tcp -m tcp --dport 5432 -j DROP")
	complexRule := chainOf("-A DOCKER-USER -i eth0 -p tcp -m tcp --dport 5432 -j DROP")
	singleSrcDrop := chainOf("-A DOCKER-USER -s 1.2.3.4/32 -p tcp -m tcp --dport 5432 -j DROP")
	tests := []struct {
		name  string
		binds []string
		chain Chain
		want  Class
	}{
		{"loopback", []string{"127.0.0.1"}, Chain{Readable: true}, ClassLoopback},
		{"loopback v6", []string{"::1"}, unreadable, ClassLoopback},
		{"private", []string{"10.1.2.3"}, Chain{Readable: true}, ClassPrivate},
		{"cgnat mesh", []string{"100.64.0.9"}, Chain{Readable: true}, ClassPrivate},
		{"wildcard no rule", []string{"0.0.0.0", "::"}, Chain{Readable: true}, ClassExposed},
		{"specific public ip", []string{"203.0.113.7"}, Chain{Readable: true}, ClassExposed},
		{"wildcard unreadable", []string{"0.0.0.0"}, unreadable, ClassUnknown},
		{"loopback plus wildcard", []string{"127.0.0.1", "0.0.0.0"}, Chain{Readable: true}, ClassExposed},
		{"allow list on host port", []string{"0.0.0.0"}, dropAll, ClassRestricted},
		{"negated drop on container port", []string{"0.0.0.0"}, containerPortDrop, ClassRestricted},
		{"interface match is not trusted", []string{"0.0.0.0"}, complexRule, ClassUnknown},
		{"drop of one source leaves it exposed", []string{"0.0.0.0"}, singleSrcDrop, ClassExposed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.binds, 5432, 5432, "tcp", tc.chain, testPrefix)
			if got.Class != tc.want {
				t.Fatalf("class = %s, want %s (%s)", got.Class, tc.want, got.Reason)
			}
		})
	}
}

func TestClassifyManagedAndAllowed(t *testing.T) {
	c := chainOf(
		"-A DOCKER-USER -s 10.0.0.0/8 -p tcp -m conntrack --ctorigdstport 8108 --ctdir ORIGINAL -m comment --comment acme:exposure:tcp/8108 -j RETURN",
		"-A DOCKER-USER -p tcp -m conntrack --ctorigdstport 8108 --ctdir ORIGINAL -m comment --comment acme:exposure:tcp/8108 -j DROP")
	v := Classify([]string{"0.0.0.0"}, 8108, 8108, "tcp", c, testPrefix)
	if v.Class != ClassRestricted || !v.Managed || len(v.Allowed) != 1 || v.Allowed[0] != "10.0.0.0/8" {
		t.Fatalf("verdict = %+v", v)
	}
	if other := Classify([]string{"0.0.0.0"}, 8109, 8109, "tcp", c, testPrefix); other.Class != ClassExposed {
		t.Fatalf("other port class = %s", other.Class)
	}
}

func TestSeverityByImageKind(t *testing.T) {
	tests := []struct {
		name  string
		class Class
		kind  ImageKind
		owner Owner
		want  Severity
	}{
		{"search engine unmanaged", ClassExposed, KindSearch, Owner{Kind: OwnerUnmanaged}, SeverityHigh},
		{"managed db not public", ClassExposed, KindDatastore, Owner{Kind: OwnerDatabase}, SeverityHigh},
		{"managed db opted in", ClassExposed, KindDatastore, Owner{Kind: OwnerDatabase, Intentional: true}, SeverityMedium},
		{"app web port", ClassExposed, KindWeb, Owner{Kind: OwnerApp}, SeverityInfo},
		{"unmanaged web", ClassExposed, KindWeb, Owner{Kind: OwnerUnmanaged}, SeverityLow},
		{"unmanaged other", ClassExposed, KindOther, Owner{Kind: OwnerUnmanaged}, SeverityMedium},
		{"unknown datastore", ClassUnknown, KindDatastore, Owner{Kind: OwnerUnmanaged}, SeverityMedium},
		{"restricted", ClassRestricted, KindDatastore, Owner{Kind: OwnerUnmanaged}, SeverityInfo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SeverityFor(tc.class, tc.kind, tc.owner); got != tc.want {
				t.Fatalf("severity = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestImageKindOf(t *testing.T) {
	tests := map[string]ImageKind{
		"typesense/typesense:27.1":            KindSearch,
		"docker.io/library/postgres@sha256:a": KindDatastore,
		"ghcr.io/acme/mystery:1":              KindOther,
		"nginx:alpine":                        KindWeb,
	}
	for image, want := range tests {
		if got := ImageKindOf(image, 0); got != want {
			t.Errorf("%s = %s, want %s", image, got, want)
		}
	}
	if ImageKindOf("acme/x", 8108) != KindSearch {
		t.Error("port fallback failed")
	}
}

func TestAuditMergesDualStackAndSkipsStopped(t *testing.T) {
	cs := []Container{
		{ID: "a", Name: "ts", Image: "typesense/typesense", Running: true, Owner: Owner{Kind: OwnerUnmanaged},
			Ports: []PortBinding{{HostIP: "0.0.0.0", HostPort: 8108, ContainerPort: 8108, Protocol: "tcp"}, {HostIP: "::", HostPort: 8108, ContainerPort: 8108, Protocol: "tcp"}}},
		{ID: "b", Name: "stopped", Image: "redis", Running: false, Ports: []PortBinding{{HostIP: "0.0.0.0", HostPort: 6379, ContainerPort: 6379}}},
		{ID: "c", Name: "loop", Image: "redis", Running: true, Ports: []PortBinding{{HostIP: "127.0.0.1", HostPort: 6380, ContainerPort: 6379, Protocol: "tcp"}}},
	}
	got := Audit(cs, Chain{Readable: true}, testPrefix)
	if len(got) != 2 {
		t.Fatalf("findings = %d, want 2: %+v", len(got), got)
	}
	if got[0].Container != "ts" || got[0].Class != ClassExposed || got[0].Severity != SeverityHigh || len(got[0].Binds) != 2 {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].Class != ClassLoopback {
		t.Fatalf("second = %+v", got[1])
	}
}

// fakeChain is a stateful iptables stand-in: -I inserts, -D removes, -S lists.
type fakeChain struct {
	rules []string
	calls [][]string
}

func (f *fakeChain) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	a := args[1:]
	switch a[0] {
	case "-S":
		out := "-N DOCKER-USER\n"
		for _, r := range f.rules {
			out += "-A DOCKER-USER " + r + "\n"
		}
		return []byte(out), nil
	case "-I":
		spec := render(a[3:])
		f.rules = append([]string{spec}, f.rules...)
		return nil, nil
	case "-D":
		spec := render(a[2:])
		for i, r := range f.rules {
			if r == spec {
				f.rules = append(f.rules[:i], f.rules[i+1:]...)
				return nil, nil
			}
		}
		return []byte("no match"), errors.New("exit 1")
	}
	return nil, errors.New("unexpected")
}

func render(tokens []string) string { return strings.Join(tokens, " ") }

func newTestManager(f *fakeChain) *Manager {
	return NewFakeManager(testPrefix, []int{22, 80, 443, 8080, 9443}, f.run,
		func(string) (string, error) { return "/sbin/iptables", nil }, "linux")
}

func TestApplyIdempotentAndOrdered(t *testing.T) {
	f := &fakeChain{rules: []string{"-j RETURN", "-s 9.9.9.9/32 -p tcp -m tcp --dport 22 -j ACCEPT"}}
	m := newTestManager(f)
	r := Restriction{Port: 8108, Allow: []string{"10.0.0.2", "192.168.0.0/16"}}
	changed, err := m.Apply(context.Background(), r)
	if err != nil || !changed {
		t.Fatalf("first apply changed=%v err=%v", changed, err)
	}
	if len(f.rules) != 5 || !strings.Contains(f.rules[2], "-j DROP") && !strings.HasSuffix(f.rules[2], "DROP") {
		t.Fatalf("rules = %v", f.rules)
	}
	for _, i := range []int{0, 1} {
		if !strings.HasSuffix(f.rules[i], "-j RETURN") {
			t.Fatalf("rule %d not RETURN before DROP: %v", i, f.rules)
		}
	}
	before := len(f.calls)
	changed, err = m.Apply(context.Background(), r)
	if err != nil || changed {
		t.Fatalf("second apply changed=%v err=%v", changed, err)
	}
	for _, c := range f.calls[before:] {
		if c[1] != "-S" {
			t.Fatalf("idempotent apply mutated: %v", c)
		}
	}
}

func TestRemoveOnlyTouchesTaggedRules(t *testing.T) {
	f := &fakeChain{rules: []string{"-j RETURN", "-p tcp -m tcp --dport 8108 -j DROP"}}
	m := newTestManager(f)
	if _, err := m.Apply(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.0/8"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(context.Background(), 8108, "tcp"); err != nil {
		t.Fatal(err)
	}
	if len(f.rules) != 2 || f.rules[1] != "-p tcp -m tcp --dport 8108 -j DROP" {
		t.Fatalf("operator rules disturbed: %v", f.rules)
	}
}

func TestRefusesLockout(t *testing.T) {
	m := newTestManager(&fakeChain{})
	for _, port := range []int{22, 80, 443, 8080, 9443} {
		if _, err := m.Plan(Restriction{Port: port, Allow: []string{"10.0.0.0/8"}}); !errors.Is(err, ErrLockout) {
			t.Errorf("port %d: err = %v, want ErrLockout", port, err)
		}
	}
	bad := []Restriction{
		{Port: 8108},
		{Port: 8108, Allow: []string{"0.0.0.0/0"}},
		{Port: 8108, Allow: []string{"nope"}},
		{Port: 8108, Allow: []string{"::1"}},
		{Port: 0, Allow: []string{"10.0.0.1"}},
	}
	for _, r := range bad {
		if _, err := m.Plan(r); err == nil {
			t.Errorf("plan accepted %+v", r)
		}
	}
}

func TestPlanRendersExactRules(t *testing.T) {
	m := newTestManager(&fakeChain{})
	p, err := m.Plan(Restriction{Port: 8108, Allow: []string{"10.0.0.2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Commands) != 2 || !strings.Contains(p.Commands[0], "-j DROP") || !strings.Contains(p.Commands[1], "-s 10.0.0.2/32") {
		t.Fatalf("commands = %v", p.Commands)
	}
	if !strings.Contains(p.Commands[0], "--comment acme:exposure:tcp/8108") {
		t.Fatalf("missing tag: %v", p.Commands[0])
	}
}

func TestSyncRemovesStaleOnly(t *testing.T) {
	f := &fakeChain{rules: []string{"-p tcp -m conntrack --ctorigdstport 9000 --ctdir ORIGINAL -m comment --comment acme:exposure:tcp/9000 -j DROP", "-j RETURN"}}
	m := newTestManager(f)
	res := m.Sync(context.Background(), []Restriction{{Port: 8108, Allow: []string{"10.0.0.0/8"}}})
	if res.Applied != 1 || res.Removed != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %+v", res)
	}
	for _, r := range f.rules {
		if strings.Contains(r, "9000") {
			t.Fatalf("stale rule left: %v", f.rules)
		}
	}
}

func TestUnreadableOffLinux(t *testing.T) {
	m := NewFakeManager(testPrefix, nil, nil, nil, "darwin")
	if c := m.ReadChain(context.Background()); c.Readable || c.Reason == "" {
		t.Fatalf("chain = %+v", c)
	}
	if _, err := m.Apply(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.0/8"}}); err == nil {
		t.Fatal("apply succeeded off linux")
	}
}
