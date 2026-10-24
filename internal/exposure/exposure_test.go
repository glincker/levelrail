package exposure

import (
	"context"
	"errors"
	"slices"
	"strconv"
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
	// failOn, when set, returns an error for a matching mutating call (before it applies).
	failOn func(args []string) error
	// history is a snapshot of rules after every mutating call.
	history [][]string
}

func (f *fakeChain) snapshot() { f.history = append(f.history, append([]string(nil), f.rules...)) }

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
		if f.failOn != nil {
			if err := f.failOn(args); err != nil {
				return []byte("boom"), err
			}
		}
		pos, _ := strconv.Atoi(a[2])
		spec := render(a[3:])
		f.rules = slices.Insert(f.rules, pos-1, spec)
		f.snapshot()
		return nil, nil
	case "-D":
		if f.failOn != nil {
			if err := f.failOn(args); err != nil {
				return []byte("boom"), err
			}
		}
		if pos, err := strconv.Atoi(a[2]); err == nil && len(a) == 3 {
			if pos < 1 || pos > len(f.rules) {
				return []byte("no match"), errors.New("exit 1")
			}
			f.rules = slices.Delete(f.rules, pos-1, pos)
			f.snapshot()
			return nil, nil
		}
		spec := render(a[2:])
		for i, r := range f.rules {
			if r == spec {
				f.rules = append(f.rules[:i], f.rules[i+1:]...)
				f.snapshot()
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
	if len(p.Commands) != 2 || !strings.Contains(p.Commands[1], "-j DROP") || !strings.Contains(p.Commands[0], "-s 10.0.0.2/32") {
		t.Fatalf("commands = %v", p.Commands)
	}
	if !strings.Contains(p.Commands[1], "--comment acme:exposure:tcp/8108") {
		t.Fatalf("missing tag: %v", p.Commands[1])
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

const (
	tagged8108 = "-m comment --comment acme:exposure:tcp/8108"
	ctPrefix   = "-p tcp -m conntrack --ctorigdstport 8108 --ctdir ORIGINAL"
)

func ownRule(source, target string) string {
	if source != "" {
		return ctPrefix + " -s " + source + " " + tagged8108 + " -j " + target
	}
	return ctPrefix + " " + tagged8108 + " -j " + target
}

func mutations(f *fakeChain) []string {
	var out []string
	for _, c := range f.calls {
		if c[1] == "-I" || c[1] == "-D" {
			out = append(out, c[1]+" "+strings.Join(c[2:], " "))
		}
	}
	return out
}

func hasDrop(rules []string) bool {
	for _, r := range rules {
		if strings.Contains(r, tagged8108) && strings.HasSuffix(r, "-j DROP") {
			return true
		}
	}
	return false
}

func TestApplyReplacesWithoutGap(t *testing.T) {
	other := "-j RETURN"
	old := []string{ownRule("10.0.0.1/32", "RETURN"), ownRule("", "DROP"), other}
	f := &fakeChain{rules: slices.Clone(old)}
	m := newTestManager(f)
	changed, err := m.Apply(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.2", "10.0.0.3"}})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	want := []string{ownRule("10.0.0.2/32", "RETURN"), ownRule("10.0.0.3/32", "RETURN"), ownRule("", "DROP"), other}
	if !slices.Equal(f.rules, want) {
		t.Fatalf("rules = %v, want %v", f.rules, want)
	}
	for i, snap := range f.history {
		if !hasDrop(snap) {
			t.Fatalf("step %d has no drop rule for the port: %v", i, snap)
		}
	}
	// New rules go in at their final positions first, then stale ones are
	// removed by position, highest first.
	wantCalls := []string{
		"-I DOCKER-USER 1 " + ctPrefix + " -s 10.0.0.2/32 " + tagged8108 + " -j RETURN",
		"-I DOCKER-USER 2 " + ctPrefix + " -s 10.0.0.3/32 " + tagged8108 + " -j RETURN",
		"-I DOCKER-USER 3 " + ctPrefix + " " + tagged8108 + " -j DROP",
		"-D DOCKER-USER 5",
		"-D DOCKER-USER 4",
	}
	gotCalls := mutations(f)
	if !slices.Equal(gotCalls, wantCalls) {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(gotCalls, "\n"), strings.Join(wantCalls, "\n"))
	}
}

func TestApplyFailureModes(t *testing.T) {
	old := []string{ownRule("10.0.0.1/32", "RETURN"), ownRule("", "DROP"), "-j RETURN"}
	req := Restriction{Port: 8108, Allow: []string{"10.0.0.2", "10.0.0.3"}}
	tests := []struct {
		name      string
		failOn    func(args []string) error
		wantRules []string
	}{
		{
			name: "insert failure keeps previous rules",
			failOn: func(a []string) error {
				if a[1] == "-I" && a[3] == "2" {
					return errors.New("exit 4")
				}
				return nil
			},
			wantRules: old,
		},
		{
			name: "first insert failure changes nothing",
			failOn: func(a []string) error {
				if a[1] == "-I" && a[3] == "1" {
					return errors.New("exit 4")
				}
				return nil
			},
			wantRules: old,
		},
		{
			name: "stale removal failure is reported and new rules stay",
			failOn: func(a []string) error {
				if a[1] == "-D" {
					return errors.New("exit 1")
				}
				return nil
			},
			wantRules: []string{
				ownRule("10.0.0.2/32", "RETURN"), ownRule("10.0.0.3/32", "RETURN"), ownRule("", "DROP"),
				ownRule("10.0.0.1/32", "RETURN"), ownRule("", "DROP"), "-j RETURN",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeChain{rules: slices.Clone(old), failOn: tc.failOn}
			m := newTestManager(f)
			if _, err := m.Apply(context.Background(), req); err == nil {
				t.Fatal("expected an error")
			}
			if !slices.Equal(f.rules, tc.wantRules) {
				t.Fatalf("rules = %v\nwant %v", f.rules, tc.wantRules)
			}
			for i, snap := range f.history {
				if !hasDrop(snap) {
					t.Fatalf("step %d has no drop rule for the port: %v", i, snap)
				}
			}
		})
	}
}

func TestApplyFinishesInterruptedCleanup(t *testing.T) {
	// New set on top, previous set left below by an earlier cut-short pass.
	f := &fakeChain{rules: []string{
		ownRule("10.0.0.2/32", "RETURN"), ownRule("", "DROP"),
		ownRule("10.0.0.1/32", "RETURN"), ownRule("", "DROP"), "-j RETURN",
	}}
	m := newTestManager(f)
	changed, err := m.Apply(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.2"}})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	want := []string{ownRule("10.0.0.2/32", "RETURN"), ownRule("", "DROP"), "-j RETURN"}
	if !slices.Equal(f.rules, want) {
		t.Fatalf("rules = %v", f.rules)
	}
	for _, c := range mutations(f) {
		if strings.HasPrefix(c, "-I") {
			t.Fatalf("cleanup pass inserted again: %s", c)
		}
	}
}

func TestApplyReapplyIsNoOp(t *testing.T) {
	f := &fakeChain{rules: []string{ownRule("10.0.0.2/32", "RETURN"), ownRule("", "DROP")}}
	m := newTestManager(f)
	changed, err := m.Apply(context.Background(), Restriction{Port: 8108, Allow: []string{"10.0.0.2"}})
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if got := mutations(f); len(got) != 0 {
		t.Fatalf("mutated: %v", got)
	}
}
