package placement

import "testing"

func regionNode(id, region, zone string, load int) Candidate {
	labels := map[string]string{}
	if region != "" {
		labels[LabelRegion] = region
	}
	if zone != "" {
		labels[LabelZone] = zone
	}
	return Candidate{ID: id, Labels: labels, Schedulable: true, Online: true, Load: load}
}

func TestSpreadPrefersDistinctRegions(t *testing.T) {
	tests := []struct {
		name   string
		env    string
		cands  []Candidate
		peers  []string
		wantID string
	}{
		{
			name:   "same zone count, other region wins over lower load",
			cands:  []Candidate{regionNode("a1", "eu", "z1", 0), regionNode("a2", "eu", "z2", 0), regionNode("b1", "us", "z3", 5)},
			peers:  []string{"a1"},
			wantID: "b1",
		},
		{
			name:   "zone still outranks region",
			cands:  []Candidate{regionNode("a1", "eu", "z1", 0), regionNode("a2", "eu", "z2", 0), regionNode("b1", "us", "z1", 0)},
			peers:  []string{"a1"},
			wantID: "a2",
		},
		{
			name:   "weight zero disables region preference",
			env:    "0",
			cands:  []Candidate{regionNode("a1", "eu", "z1", 0), regionNode("a2", "eu", "z2", 0), regionNode("b1", "us", "z3", 5)},
			peers:  []string{"a1"},
			wantID: "a2",
		},
		{
			name:   "invalid weight falls back to default",
			env:    "nope",
			cands:  []Candidate{regionNode("a1", "eu", "z1", 0), regionNode("a2", "eu", "z2", 0), regionNode("b1", "us", "z3", 5)},
			peers:  []string{"a1"},
			wantID: "b1",
		},
		{
			name:   "region-only labels spread across regions",
			cands:  []Candidate{regionNode("a1", "eu", "", 0), regionNode("a2", "eu", "", 0), regionNode("b1", "us", "", 3)},
			peers:  []string{"a1"},
			wantID: "b1",
		},
		{
			name:   "no labels keeps headroom order",
			cands:  []Candidate{regionNode("a", "", "", 2), regionNode("b", "", "", 0)},
			peers:  []string{"b"},
			wantID: "b",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv(EnvRegionSpreadWeight, tc.env)
			}
			d, err := Spread(tc.cands, Request{Peers: tc.peers})
			if err != nil {
				t.Fatal(err)
			}
			if d.NodeID != tc.wantID {
				t.Fatalf("chose %s, want %s", d.NodeID, tc.wantID)
			}
		})
	}
}
