package migrationgate

import "testing"

func green() Input {
	return Input{
		Items: []Item{
			{Name: "web", State: "routed", Domains: []string{"app.example.com"}, Volumes: []Volume{{Name: "data", Copied: true}}},
			{Name: "worker", State: "verified"},
		},
		Databases:    []Database{{Name: "main-db", Status: "verified"}},
		Probes:       []Probe{{Domain: "app.example.com", DNS: true, Responds: true}},
		Acknowledged: []string{GateBackup, GateTraffic},
	}
}

func gate(t *testing.T, r Report, id string) Gate {
	t.Helper()
	for _, g := range r.Gates {
		if g.ID == id {
			return g
		}
	}
	t.Fatalf("no gate %q", id)
	return Gate{}
}

func TestSafeOnlyWhenEveryGatePasses(t *testing.T) {
	if r := Evaluate(green()); !r.Safe {
		t.Fatalf("all evidence present but not safe: %+v", r.Gates)
	}
}

func TestEachBreakageBlocksTheVerdict(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Input)
		gate   string
		want   Status
	}{
		{"app not verified", func(in *Input) { in.Items[1].State = "stage-failed" }, GateStaged, Fail},
		{"no app at all", func(in *Input) { in.Items = nil }, GateStaged, Fail},
		{"unsupported leftover", func(in *Input) { in.Items = append(in.Items, Item{Name: "plausible", Leftover: true}) }, GateLeftovers, Manual},
		{"volume not copied", func(in *Input) { in.Items[0].Volumes[0].Copied = false }, GateVolumes, Fail},
		{"database never copied", func(in *Input) { in.Databases[0].Status = "" }, GateDatabases, Fail},
		{"database tables differ", func(in *Input) { in.Databases[0].Mismatched = 2 }, GateDatabases, Fail},
		{"domain not routed", func(in *Input) { in.Items[0].State = "verified" }, GateRouted, Fail},
		{"dns still old", func(in *Input) { in.Probes[0].DNS = false }, GateDNS, Fail},
		{"app does not answer", func(in *Input) { in.Probes[0].Responds = false }, GateResponds, Fail},
		{"probes not run", func(in *Input) { in.Probes = nil }, GateDNS, Pending},
		{"no backup confirmation", func(in *Input) { in.Acknowledged = []string{GateTraffic} }, GateBackup, Manual},
		{"no traffic cycle confirmation", func(in *Input) { in.Acknowledged = []string{GateBackup} }, GateTraffic, Manual},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := green()
			c.mutate(&in)
			r := Evaluate(in)
			if r.Safe {
				t.Fatalf("reported safe to delete")
			}
			if got := gate(t, r, c.gate).Status; got != c.want {
				t.Fatalf("gate %s = %s, want %s", c.gate, got, c.want)
			}
		})
	}
}

func TestAcknowledgingLeftoversClearsOnlyThatGate(t *testing.T) {
	in := green()
	in.Items = append(in.Items, Item{Name: "plausible", Leftover: true})
	in.Acknowledged = append(in.Acknowledged, GateLeftovers)
	if r := Evaluate(in); !r.Safe {
		t.Fatalf("acknowledged leftovers should clear: %+v", r.Gates)
	}
}

func TestProbeDomainMatchIsCaseInsensitive(t *testing.T) {
	in := green()
	in.Probes[0].Domain = "APP.example.com"
	if r := Evaluate(in); !r.Safe {
		t.Fatalf("case differences must not fail a probe: %+v", r.Gates)
	}
}

func TestNoDomainsNeedNoProbes(t *testing.T) {
	in := Input{Items: []Item{{Name: "cron", State: "verified"}}, Acknowledged: []string{GateBackup, GateTraffic}}
	if r := Evaluate(in); !r.Safe {
		t.Fatalf("an app without domains should not wait on probes: %+v", r.Gates)
	}
}

func TestFallbackGuidanceAlwaysPresent(t *testing.T) {
	if len(Evaluate(Input{}).Fallback) == 0 {
		t.Fatal("rollback guidance missing")
	}
}
