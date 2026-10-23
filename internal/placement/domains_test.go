package placement

import (
	"reflect"
	"testing"
)

func domainNode(id, provider, region, zone string, load int, online bool) Candidate {
	labels := map[string]string{}
	for k, v := range map[string]string{LabelProvider: provider, LabelRegion: region, LabelZone: zone} {
		if v != "" {
			labels[k] = v
		}
	}
	return Candidate{ID: id, Labels: labels, Schedulable: true, Online: online, Load: load,
		Capacity: Capacity{CPUMilli: Measured(4000), MemoryBytes: Measured(8 << 30), DiskFreeBytes: Measured(50 << 30)}}
}

func multiCloud() []Candidate {
	return []Candidate{
		domainNode("aws1", "aws", "us-east-1", "us-east-1a", 2, true),
		domainNode("aws2", "aws", "us-east-1", "us-east-1b", 1, true),
		domainNode("hz1", "hetzner", "eu-central", "fsn1-dc14", 0, true),
		domainNode("home", "", "", "", 3, false),
	}
}

func TestDomainsGroupsCapacityByProviderRegionZone(t *testing.T) {
	o := Domains(multiCloud())
	if o.Total != 4 || o.Unlabeled != 1 {
		t.Fatalf("total=%d unlabeled=%d", o.Total, o.Unlabeled)
	}
	names := func(g []DomainGroup) []string {
		var out []string
		for _, d := range g {
			out = append(out, d.Name)
		}
		return out
	}
	if got := names(o.Providers); !reflect.DeepEqual(got, []string{"aws", "hetzner"}) {
		t.Errorf("providers = %v", got)
	}
	if got := names(o.Regions); !reflect.DeepEqual(got, []string{"eu-central", "us-east-1"}) {
		t.Errorf("regions = %v", got)
	}
	if got := names(o.Zones); !reflect.DeepEqual(got, []string{"eu-central/fsn1-dc14", "us-east-1/us-east-1a", "us-east-1/us-east-1b"}) {
		t.Errorf("zones = %v", got)
	}
	aws := o.Providers[0]
	if aws.Nodes != 2 || aws.Workloads != 3 || aws.CPUMilli != 8000 || aws.MemoryBytes != 16<<30 || aws.Unverified != 0 {
		t.Errorf("aws = %+v", aws)
	}
	if !reflect.DeepEqual(o.Regions[1].Providers, []string{"aws"}) {
		t.Errorf("region providers = %v", o.Regions[1].Providers)
	}
}

func TestDomainsCountsOfflineAndUnverifiedHonestly(t *testing.T) {
	c := domainNode("n1", "aws", "r", "z", 0, false)
	c.Capacity = Capacity{}
	o := Domains([]Candidate{c, domainNode("n2", "aws", "r", "z", 0, true)})
	g := o.Providers[0]
	if g.Nodes != 2 || g.Online != 1 || g.Schedulable != 1 || g.Unverified != 1 || g.CPUMilli != 4000 {
		t.Errorf("group = %+v", g)
	}
}

func TestDomainsSameZoneNameInTwoRegionsStaysSeparate(t *testing.T) {
	o := Domains([]Candidate{domainNode("a", "p", "eu", "a", 0, true), domainNode("b", "p", "us", "a", 0, true)})
	if len(o.Zones) != 2 {
		t.Fatalf("zones = %+v", o.Zones)
	}
}

func TestDomainRisksFlagsReplicasInOneZone(t *testing.T) {
	cands := multiCloud()
	risks := DomainRisks(cands, []RiskInput{
		{Name: "web", NodeID: "aws1", Replicas: 3},
		{Name: "solo", NodeID: "aws1", Replicas: 1},
		{Name: "db", NodeID: "hz1", Replicas: 2},
		{Name: "ghost", NodeID: "missing", Replicas: 2},
	})
	var got []string
	for _, r := range risks {
		got = append(got, r.Name+":"+r.Kind+":"+r.Level)
	}
	want := []string{
		"db:replicas_one_provider:info",
		"db:replicas_one_zone:warn",
		"web:replicas_one_provider:info",
		"web:replicas_one_zone:warn",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("risks = %v, want %v", got, want)
	}
	for _, r := range risks {
		if r.Name == "web" && r.Kind == RiskReplicasOneZone {
			if !reflect.DeepEqual(r.Alternatives, []string{"fsn1-dc14", "us-east-1b"}) || r.Replicas != 3 {
				t.Errorf("web zone risk = %+v", r)
			}
		}
	}
}

func TestDomainRisksNeedsAnEligibleAlternative(t *testing.T) {
	tests := []struct {
		name  string
		mod   func([]Candidate) []Candidate
		input RiskInput
		want  int
	}{
		{"only one zone exists", func(c []Candidate) []Candidate { return c[:1] }, RiskInput{Name: "w", NodeID: "aws1", Replicas: 2}, 0},
		{"other zone offline", func(c []Candidate) []Candidate {
			c[1].Online, c[2].Online = false, false
			return c
		}, RiskInput{Name: "w", NodeID: "aws1", Replicas: 2}, 0},
		{"other zone cordoned", func(c []Candidate) []Candidate {
			c[1].Schedulable, c[2].Schedulable = false, false
			return c
		}, RiskInput{Name: "w", NodeID: "aws1", Replicas: 2}, 0},
		{"selector excludes the alternatives", func(c []Candidate) []Candidate { return c },
			RiskInput{Name: "w", NodeID: "aws1", Replicas: 2, Selector: Selector{LabelProvider: "aws", LabelZone: "us-east-1a"}}, 0},
		{"home node unlabeled", func(c []Candidate) []Candidate { return c }, RiskInput{Name: "w", NodeID: "home", Replicas: 2}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DomainRisks(tc.mod(multiCloud()), []RiskInput{tc.input}); len(got) != tc.want {
				t.Errorf("risks = %+v, want %d", got, tc.want)
			}
		})
	}
}

func TestDomainRisksDeterministicOrder(t *testing.T) {
	in := []RiskInput{{Name: "z", NodeID: "aws1", Replicas: 2}, {Name: "a", NodeID: "aws1", Replicas: 2}, {Name: "m", NodeID: "aws1", Replicas: 2}}
	first := DomainRisks(multiCloud(), in)
	for range 10 {
		if got := DomainRisks(multiCloud(), in); !reflect.DeepEqual(got, first) {
			t.Fatal("risk order changed between runs")
		}
	}
	if first[0].Name != "a" {
		t.Errorf("first = %+v", first[0])
	}
}

func provNode(id, provider, zone string, load int) Candidate {
	return domainNode(id, provider, "", zone, load, true)
}

func TestSpreadPrefersAnotherProviderWhenZonesAreUnlabeled(t *testing.T) {
	cands := []Candidate{provNode("aws1", "aws", "", 0), provNode("aws2", "aws", "", 0), provNode("hz1", "hetzner", "", 4)}
	d, err := Spread(cands, Request{Peers: []string{"aws1"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.NodeID != "hz1" {
		t.Errorf("chose %s, want the other provider despite its higher load", d.NodeID)
	}
}

func TestSpreadZoneStillOutranksProvider(t *testing.T) {
	cands := []Candidate{provNode("aws1", "aws", "a", 0), provNode("aws2", "aws", "b", 0), provNode("hz1", "hetzner", "a", 0)}
	d, err := Spread(cands, Request{Peers: []string{"aws1"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.NodeID != "aws2" {
		t.Errorf("chose %s, want the empty zone first", d.NodeID)
	}
}

func TestSpreadProviderWeightZeroDisablesPreference(t *testing.T) {
	t.Setenv(EnvProviderSpreadWeight, "0")
	cands := []Candidate{provNode("aws1", "aws", "", 0), provNode("aws2", "aws", "", 0), provNode("hz1", "hetzner", "", 4)}
	d, err := Spread(cands, Request{Peers: []string{"aws1"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.NodeID == "hz1" {
		t.Errorf("chose %s, want the least loaded node when provider spread is off", d.NodeID)
	}
}

func TestSpreadAcrossProvidersIsDeterministicAndBalanced(t *testing.T) {
	run := func() map[string]int {
		cands := []Candidate{
			provNode("aws1", "aws", "", 0), provNode("aws2", "aws", "", 0),
			provNode("hz1", "hetzner", "", 0), provNode("hz2", "hetzner", "", 0),
			provNode("do1", "digitalocean", "", 0),
		}
		placed := placeReplicas(t, cands, 6)
		out := map[string]int{}
		byID := map[string]string{}
		for _, c := range cands {
			byID[c.ID] = c.Labels[LabelProvider]
		}
		for _, id := range placed {
			out[byID[id]]++
		}
		return out
	}
	first := run()
	for _, n := range first {
		if n < 1 || n > 3 {
			t.Errorf("unbalanced across providers: %v", first)
		}
	}
	for range 5 {
		if got := run(); !reflect.DeepEqual(got, first) {
			t.Fatalf("placement is not deterministic: %v vs %v", got, first)
		}
	}
}

func TestSpreadNeverPicksOfflineNodeForProviderDiversity(t *testing.T) {
	hz := provNode("hz1", "hetzner", "", 0)
	hz.Online = false
	cands := []Candidate{provNode("aws1", "aws", "", 0), provNode("aws2", "aws", "", 0), hz}
	d, err := Spread(cands, Request{Peers: []string{"aws1"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.NodeID == "hz1" {
		t.Error("an offline node must never win for diversity")
	}
}
