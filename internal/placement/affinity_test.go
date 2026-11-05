package placement

import (
	"strings"
	"testing"
)

func zoned(id, zone string, workloads ...map[string]string) Candidate {
	c := capNode(id, 0)
	c.Labels = map[string]string{"zone": zone}
	c.Workloads = workloads
	return c
}

func TestLabelTermMatches(t *testing.T) {
	labels := map[string]string{"disk": "ssd", "tier": "web"}
	tests := []struct {
		term LabelTerm
		want bool
	}{
		{LabelTerm{Key: "disk", Op: OpIn, Values: []string{"ssd", "nvme"}}, true},
		{LabelTerm{Key: "disk", Op: OpIn, Values: []string{"hdd"}}, false},
		{LabelTerm{Key: "disk", Op: OpNotIn, Values: []string{"hdd"}}, true},
		{LabelTerm{Key: "gpu", Op: OpNotIn, Values: []string{"a100"}}, true},
		{LabelTerm{Key: "tier", Op: OpExists}, true},
		{LabelTerm{Key: "gpu", Op: OpExists}, false},
		{LabelTerm{Key: "gpu", Op: OpDoesNotExist}, true},
		{LabelTerm{Key: "disk", Op: "Gt"}, false},
	}
	for _, tt := range tests {
		if got := tt.term.Matches(labels); got != tt.want {
			t.Errorf("%s = %v, want %v", tt.term, got, tt.want)
		}
	}
}

func TestNodeAffinity(t *testing.T) {
	ssd := capNode("ssd", 3000)
	ssd.Labels = map[string]string{"disk": "ssd"}
	hdd := capNode("hdd", 0)
	hdd.Labels = map[string]string{"disk": "hdd"}

	d, err := Spread([]Candidate{ssd, hdd}, Request{Affinity: Affinity{NodeRequired: []LabelTerm{{Key: "disk", Op: OpIn, Values: []string{"ssd"}}}}})
	if err != nil || d.NodeID != "ssd" {
		t.Fatalf("required node affinity filters: %+v %v", d, err)
	}
	d, err = Spread([]Candidate{ssd, hdd}, Request{Affinity: Affinity{NodePreferred: []WeightedNodeTerm{{Weight: 10, Term: LabelTerm{Key: "disk", Op: OpIn, Values: []string{"ssd"}}}}}})
	if err != nil || d.NodeID != "ssd" {
		t.Fatalf("a preference outranks headroom: %+v %v", d, err)
	}
	d, err = Spread([]Candidate{hdd}, Request{Affinity: Affinity{NodePreferred: []WeightedNodeTerm{{Weight: 10, Term: LabelTerm{Key: "disk", Op: OpIn, Values: []string{"ssd"}}}}}})
	if err != nil || d.NodeID != "hdd" {
		t.Fatalf("a preference never filters: %+v %v", d, err)
	}
}

func TestWorkloadAffinityAndAntiAffinity(t *testing.T) {
	db := map[string]string{"app": "db"}
	web := map[string]string{"app": "web"}
	a := zoned("a", "eu-1", db)
	b := zoned("b", "eu-1")
	c := zoned("c", "eu-2", web)
	cands := []Candidate{a, b, c}
	match := func(app string) []LabelTerm { return []LabelTerm{{Key: "app", Op: OpIn, Values: []string{app}}} }

	d, err := Spread(cands, Request{Affinity: Affinity{Together: []WorkloadTerm{{Match: match("db")}}}})
	if err != nil || d.NodeID != "a" {
		t.Fatalf("required affinity per node picks the db node: %+v %v", d, err)
	}
	d, err = Spread(cands, Request{Affinity: Affinity{Together: []WorkloadTerm{{Match: match("db"), Topology: "zone"}}}})
	if err != nil || (d.NodeID != "a" && d.NodeID != "b") {
		t.Fatalf("required affinity per zone allows any eu-1 node: %+v %v", d, err)
	}
	d, err = Spread(cands, Request{Affinity: Affinity{Apart: []WorkloadTerm{{Match: match("web"), Topology: "zone"}}}})
	if err != nil || d.NodeID == "c" {
		t.Fatalf("anti-affinity keeps the web zone clear: %+v %v", d, err)
	}
	var why string
	for _, r := range d.Rejected {
		if r.NodeID == "c" {
			why = r.Reason
		}
	}
	if !strings.Contains(why, "anti-affinity") || !strings.Contains(why, "per zone") {
		t.Fatalf("rejection should explain the rule, got %q", why)
	}

	_, err = Spread([]Candidate{c}, Request{Affinity: Affinity{Together: []WorkloadTerm{{Match: match("db")}}}})
	if err == nil {
		t.Fatal("an unsatisfiable required affinity must leave the workload unplaced, not fall back")
	}

	d, err = Spread(cands, Request{Affinity: Affinity{Together: []WorkloadTerm{{Match: match("db"), Weight: 50}}}})
	if err != nil || d.NodeID != "a" {
		t.Fatalf("preferred affinity ranks the db node first: %+v %v", d, err)
	}
	d, err = Spread([]Candidate{c}, Request{Affinity: Affinity{Together: []WorkloadTerm{{Match: match("db"), Weight: 50}}}})
	if err != nil || d.NodeID != "c" {
		t.Fatalf("preferred affinity never filters: %+v %v", d, err)
	}
}

func TestEmptyAffinityChangesNothing(t *testing.T) {
	cands := []Candidate{capNode("a", 2000), capNode("b", 0)}
	with, err := Spread(cands, Request{Affinity: Affinity{}})
	if err != nil || with.NodeID != "b" {
		t.Fatalf("%+v %v", with, err)
	}
}

func TestValidateAffinity(t *testing.T) {
	good := Affinity{
		NodeRequired:  []LabelTerm{{Key: "disk", Op: OpIn, Values: []string{"ssd"}}},
		NodePreferred: []WeightedNodeTerm{{Weight: 5, Term: LabelTerm{Key: "gpu", Op: OpExists}}},
		Together:      []WorkloadTerm{{Match: []LabelTerm{{Key: "app", Op: OpIn, Values: []string{"db"}}}, Topology: "zone"}},
		Apart:         []WorkloadTerm{{Match: []LabelTerm{{Key: "app", Op: OpExists}}, Weight: 10}},
	}
	if err := ValidateAffinity(good, 8); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAffinity(good, 3); err == nil {
		t.Error("term cap should apply")
	}
	bad := []Affinity{
		{NodeRequired: []LabelTerm{{Key: "disk", Op: OpIn}}},
		{NodeRequired: []LabelTerm{{Key: "disk", Op: OpExists, Values: []string{"x"}}}},
		{NodeRequired: []LabelTerm{{Key: "Bad Key", Op: OpExists}}},
		{NodePreferred: []WeightedNodeTerm{{Weight: 0, Term: LabelTerm{Key: "a", Op: OpExists}}}},
		{Together: []WorkloadTerm{{}}},
		{Apart: []WorkloadTerm{{Match: []LabelTerm{{Key: "a", Op: OpExists}}, Weight: 101}}},
		{Apart: []WorkloadTerm{{Match: []LabelTerm{{Key: "a", Op: OpExists}}, Topology: "Not A Key"}}},
	}
	for i, a := range bad {
		if err := ValidateAffinity(a, 0); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}
