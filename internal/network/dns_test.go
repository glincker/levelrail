package network

import (
	"net/netip"
	"strings"
	"testing"
)

func TestZone(t *testing.T) {
	tests := []struct {
		name      string
		shortName string
		want      string
	}{
		{name: "simple", shortName: "acme", want: "acme.internal"},
		{name: "mixed case is lowercased", shortName: "AcMe", want: "acme.internal"},
		{name: "spaces and punctuation are stripped", shortName: "Acme Corp.", want: "acmecorp.internal"},
		{name: "hyphens are kept inside", shortName: "acme-two", want: "acme-two.internal"},
		{name: "leading and trailing hyphens are trimmed", shortName: "-acme-", want: "acme.internal"},
		{name: "empty falls back to a non branded label", shortName: "", want: "mesh.internal"},
		{name: "all punctuation falls back too", shortName: "!!!", want: "mesh.internal"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Zone(tc.shortName); got != tc.want {
				t.Errorf("Zone(%q) = %q, want %q", tc.shortName, got, tc.want)
			}
		})
	}
}

func TestZone_FollowsTheShortName(t *testing.T) {
	// CLAUDE.md section 3: a rebrand has to change the zone, so the zone
	// cannot be a constant here. The zone is also user-visible (it is in
	// every connection string), which is why this matters beyond the rule.
	if Zone("brandone") == Zone("brandtwo") {
		t.Fatal("the DNS zone does not follow the brand short name")
	}
}

func meshNodes() []NodeInfo {
	return []NodeInfo{
		{ID: "control", Name: "control", PublicKey: testKey(1), Address: netip.MustParseAddr("10.181.0.1")},
		{ID: "worker", Name: "worker", PublicKey: testKey(2), Address: netip.MustParseAddr("10.181.0.2")},
	}
}

// TestBuildRecords_MovedServiceKeepsItsName is TASKS.md 3.4's whole
// reason for existing, and Phase 3's exit criterion in one assertion: a
// connection string is a literal string in a running container's
// environment, nothing rewrites it when a service moves, so the name has
// to resolve to wherever the service is now.
func TestBuildRecords_MovedServiceKeepsItsName(t *testing.T) {
	zone := Zone("acme")
	nodes := meshNodes()

	before, err := BuildRecords(zone, "control", []Placement{{Service: "postgres", NodeID: ""}}, nodes)
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	resolver := NewResolver(zone)
	resolver.SetRecords(before)

	addr, ok := resolver.Lookup("postgres." + zone)
	if !ok {
		t.Fatal("postgres does not resolve before the move")
	}
	if addr.String() != "10.181.0.1" {
		t.Fatalf("postgres resolves to %s, want the control plane's own node", addr)
	}

	// The move: the same service, now placed on the other node. This is
	// exactly what TASKS.md 3.3's PUT /apps/{name}/node does.
	after, err := BuildRecords(zone, "control", []Placement{{Service: "postgres", NodeID: "worker"}}, nodes)
	if err != nil {
		t.Fatalf("BuildRecords after the move: %v", err)
	}
	resolver.SetRecords(after)

	addr, ok = resolver.Lookup("postgres." + zone)
	if !ok {
		t.Fatal("postgres stopped resolving after the move: every connection string holding this name is now broken")
	}
	if addr.String() != "10.181.0.2" {
		t.Errorf("postgres resolves to %s after the move, want the worker's mesh address 10.181.0.2", addr)
	}
}

func TestBuildRecords_NameFamilies(t *testing.T) {
	zone := Zone("acme")
	set, err := BuildRecords(zone, "control", []Placement{
		{Service: "web", NodeID: "worker"},
		{Service: "postgres"},
	}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}

	want := map[string]string{
		"web." + zone:          "10.181.0.2",
		"postgres." + zone:     "10.181.0.1",
		"control.node." + zone: "10.181.0.1",
		"worker.node." + zone:  "10.181.0.2",
	}
	if len(set.Records) != len(want) {
		t.Fatalf("got %d records, want %d: %+v", len(set.Records), len(want), set.Records)
	}
	for _, rec := range set.Records {
		wantAddr, known := want[rec.Name]
		if !known {
			t.Errorf("unexpected record %q", rec.Name)
			continue
		}
		if rec.Address.String() != wantAddr {
			t.Errorf("%q resolves to %s, want %s", rec.Name, rec.Address, wantAddr)
		}
	}
}

// TestBuildRecords_NodeNamesCannotCollideWithServiceNames is why node
// records sit under a .node label: "db" is a genuinely likely name for
// both a machine and a database.
func TestBuildRecords_NodeNamesCannotCollideWithServiceNames(t *testing.T) {
	zone := Zone("acme")
	nodes := []NodeInfo{
		{ID: "n1", Name: "db", PublicKey: testKey(1), Address: netip.MustParseAddr("10.181.0.1")},
	}
	set, err := BuildRecords(zone, "n1", []Placement{{Service: "db"}}, nodes)
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}

	resolver := NewResolver(zone)
	resolver.SetRecords(set)
	if _, ok := resolver.Lookup("db." + zone); !ok {
		t.Error("the service record is missing")
	}
	if _, ok := resolver.Lookup("db.node." + zone); !ok {
		t.Error("the node record is missing")
	}
	if len(set.Records) != 2 {
		t.Errorf("got %d records, want 2: a node and a service sharing a name must not collide", len(set.Records))
	}
}

func TestBuildRecords_UnresolvablePlacementsAreReportedNotDropped(t *testing.T) {
	zone := Zone("acme")
	nodes := []NodeInfo{
		{ID: "control", Name: "control", PublicKey: testKey(1), Address: netip.MustParseAddr("10.181.0.1")},
		{ID: "pending", Name: "pending", PublicKey: testKey(2)}, // enrolled, no address yet
	}

	set, err := BuildRecords(zone, "control", []Placement{
		{Service: "ok"},
		{Service: "on-a-pending-node", NodeID: "pending"},
		{Service: "on-a-ghost-node", NodeID: "vanished"},
	}, nodes)
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}

	// The pass still produced every record it could.
	resolver := NewResolver(zone)
	resolver.SetRecords(set)
	if _, ok := resolver.Lookup("ok." + zone); !ok {
		t.Error("a resolvable service was dropped because another was not resolvable")
	}

	if len(set.Unresolved) != 2 {
		t.Fatalf("Unresolved = %+v, want two entries", set.Unresolved)
	}
	if set.Unresolved[0].Service != "on-a-ghost-node" || !strings.Contains(set.Unresolved[0].Reason, "not in the mesh inventory") {
		t.Errorf("Unresolved[0] = %+v", set.Unresolved[0])
	}
	if set.Unresolved[1].Service != "on-a-pending-node" || !strings.Contains(set.Unresolved[1].Reason, "no mesh address") {
		t.Errorf("Unresolved[1] = %+v", set.Unresolved[1])
	}
}

func TestBuildRecords_Failures(t *testing.T) {
	tests := []struct {
		name       string
		zone       string
		placements []Placement
		want       string
	}{
		{
			name: "empty zone",
			zone: "",
			want: "zone is empty",
		},
		{
			name:       "empty service name",
			zone:       "acme.internal",
			placements: []Placement{{Service: ""}},
			want:       "empty service name",
		},
		{
			// Two services answering to one name means whichever record
			// won would silently take the other's traffic. Unlike an
			// unresolved placement there is no correct partial answer.
			name:       "duplicate service names",
			zone:       "acme.internal",
			placements: []Placement{{Service: "web"}, {Service: "WEB"}},
			want:       "both resolve to",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildRecords(tc.zone, "control", tc.placements, meshNodes())
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestBuildRecords_Deterministic(t *testing.T) {
	zone := Zone("acme")
	placements := []Placement{{Service: "z"}, {Service: "a"}, {Service: "m"}}

	first, err := BuildRecords(zone, "control", placements, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	second, err := BuildRecords(zone, "control", placements, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	for i := range first.Records {
		if first.Records[i].Name != second.Records[i].Name {
			t.Fatalf("record order is not stable at index %d", i)
		}
	}
	if first.Records[0].Name != "a."+zone {
		t.Errorf("records are not sorted by name: first is %q", first.Records[0].Name)
	}
}

func TestResolver_LookupNormalizesNames(t *testing.T) {
	zone := Zone("acme")
	set, err := BuildRecords(zone, "control", []Placement{{Service: "web"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r := NewResolver(zone)
	r.SetRecords(set)

	// A DNS query arrives fully qualified with a trailing dot; a human
	// types neither that nor consistent case.
	for _, name := range []string{"web." + zone, "web." + zone + ".", "WEB." + strings.ToUpper(zone), "Web." + zone + "."} {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("Lookup(%q) missed", name)
		}
	}
	if _, ok := r.Lookup("nothing." + zone); ok {
		t.Error("an unknown name resolved")
	}
}

func TestResolver_EmptyAnswersNothing(t *testing.T) {
	// Before the first reconcile pass, NXDOMAIN is the correct answer: a
	// stale or guessed one would be a silent misroute.
	r := NewResolver(Zone("acme"))
	if _, ok := r.Lookup("web.acme.internal"); ok {
		t.Error("an empty resolver answered a lookup")
	}
	if len(r.Records()) != 0 {
		t.Error("an empty resolver has records")
	}
}

func TestResolver_SetRecordsReplacesEverything(t *testing.T) {
	zone := Zone("acme")
	r := NewResolver(zone)

	first, err := BuildRecords(zone, "control", []Placement{{Service: "old"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(first)

	second, err := BuildRecords(zone, "control", []Placement{{Service: "new"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(second)

	if _, ok := r.Lookup("old." + zone); ok {
		t.Error("a deleted service still resolves: SetRecords is not replacing the table")
	}
	if _, ok := r.Lookup("new." + zone); !ok {
		t.Error("the new service does not resolve")
	}
}

func TestResolver_Authoritative(t *testing.T) {
	zone := Zone("acme")
	r := NewResolver(zone)

	tests := []struct {
		name string
		want bool
	}{
		{name: "web." + zone, want: true},
		{name: "web." + zone + ".", want: true},
		{name: zone, want: true},
		{name: "example.com.", want: false},
		{name: "notacme.internal.", want: false},
		// A name that merely ends in the zone's characters without a
		// label boundary is not ours.
		{name: "fake" + zone + ".", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.Authoritative(tc.name); got != tc.want {
				t.Errorf("Authoritative(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestResolver_RecordsIsACopy(t *testing.T) {
	zone := Zone("acme")
	r := NewResolver(zone)
	set, err := BuildRecords(zone, "control", []Placement{{Service: "web"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(set)

	got := r.Records()
	got[0].Address = netip.MustParseAddr("192.0.2.1")

	if addr, _ := r.Lookup(r.Records()[0].Name); addr.String() == "192.0.2.1" {
		t.Error("mutating the slice from Records() changed resolution state")
	}
}
