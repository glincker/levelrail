package exposure

import "testing"

func exposedPort(owner Owner, kind ImageKind, port int, sev Severity) Finding {
	return Finding{Container: "c1", Owner: owner, ImageKind: kind, Protocol: "tcp", HostPort: port, Class: ClassExposed, Severity: sev}
}

func TestBuildReachFindings(t *testing.T) {
	local := ReachNode{Name: "local", Local: true, RulesReadable: true}
	remote := ReachNode{Name: "edge", RulesReadable: true}
	tests := []struct {
		name     string
		node     ReachNode
		f        Finding
		wantKind ReachKind
		wantFix  string
		wantAuto bool
	}{
		{"app on local node gets an auto fix", local, exposedPort(Owner{Kind: OwnerApp, Name: "web"}, KindWeb, 24100, SeverityMedium), ReachAppDirectPort, "/api/v1/apps/web/exposure", true},
		{"app on remote node is guided", remote, exposedPort(Owner{Kind: OwnerApp, Name: "web"}, KindWeb, 24100, SeverityMedium), ReachAppDirectPort, "", false},
		{"database is guided", local, exposedPort(Owner{Kind: OwnerDatabase, Name: "main"}, KindDatastore, 5432, SeverityHigh), ReachDatabasePublic, "", false},
		{"docker api", local, exposedPort(Owner{Kind: OwnerUnmanaged}, KindDockerAPI, 2375, SeverityHigh), ReachDockerAPI, "", false},
		{"unmanaged port", local, exposedPort(Owner{Kind: OwnerUnmanaged}, KindOther, 9000, SeverityMedium), ReachPortExposed, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.node
			n.Findings = []Finding{tc.f}
			rep := BuildReach(ReachInput{Nodes: []ReachNode{n}})
			if len(rep.Findings) != 1 {
				t.Fatalf("findings = %d, want 1", len(rep.Findings))
			}
			got := rep.Findings[0]
			if got.Kind != tc.wantKind || got.Fix == nil {
				t.Fatalf("kind = %q fix = %v, want %q with a fix", got.Kind, got.Fix, tc.wantKind)
			}
			if (got.Fix.Kind == ReachFixAuto) != tc.wantAuto {
				t.Fatalf("fix kind = %q, auto want %v", got.Fix.Kind, tc.wantAuto)
			}
			if tc.wantFix != "" && got.Fix.Path != tc.wantFix {
				t.Fatalf("fix path = %q, want %q", got.Fix.Path, tc.wantFix)
			}
		})
	}
}

func TestBuildReachSkipsSafePorts(t *testing.T) {
	n := ReachNode{Name: "local", Local: true, RulesReadable: false, Findings: []Finding{
		{Owner: Owner{Kind: OwnerApp, Name: "web"}, Class: ClassLoopback, Protocol: "tcp", HostPort: 3000},
		{Owner: Owner{Kind: OwnerDatabase, Name: "main"}, Class: ClassRestricted, Severity: SeverityInfo, Protocol: "tcp", HostPort: 5432},
		exposedPort(Owner{Kind: OwnerUnmanaged}, KindOther, 8080, SeverityInfo),
	}}
	rep := BuildReach(ReachInput{Nodes: []ReachNode{n}})
	if len(rep.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", rep.Findings)
	}
	if rep.RestrictedPorts != 1 || rep.RulesReadable {
		t.Fatalf("restricted = %d readable = %v", rep.RestrictedPorts, rep.RulesReadable)
	}
	var db *ReachSubject
	for i := range rep.Subjects {
		if rep.Subjects[i].Type == SubjectDatabase {
			db = &rep.Subjects[i]
		}
	}
	if db == nil || len(db.Endpoints) != 1 || db.Endpoints[0].Class != ClassRestricted {
		t.Fatalf("database subject = %+v", db)
	}
}

func TestBuildReachShieldAndOrdering(t *testing.T) {
	rep := BuildReach(ReachInput{
		Apps: []ReachApp{{Name: "web", Domains: []string{"a.example.com", "b.example.com"}, Unshielded: []string{"b.example.com"}}},
		Nodes: []ReachNode{{Name: "local", Local: true, RulesReadable: true,
			Findings: []Finding{exposedPort(Owner{Kind: OwnerDatabase, Name: "main"}, KindDatastore, 5432, SeverityHigh)}}},
	})
	if len(rep.Findings) != 2 || rep.Findings[0].Severity != SeverityHigh || rep.Findings[1].Kind != ReachAppNoShield {
		t.Fatalf("findings = %+v", rep.Findings)
	}
	if f := rep.Findings[1].Fix; f == nil || f.Kind != ReachFixAuto || f.Path != "/api/v1/apps/web/shield/recommended" {
		t.Fatalf("shield fix = %+v", f)
	}
	if rep.Counts.High != 1 || rep.Counts.Low != 1 {
		t.Fatalf("counts = %+v", rep.Counts)
	}
	if len(rep.Subjects[0].Endpoints) == 0 && len(rep.Subjects[1].Endpoints) == 0 {
		t.Fatal("expected app domains as endpoints")
	}
}
