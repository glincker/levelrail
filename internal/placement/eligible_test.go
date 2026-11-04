package placement

import (
	"reflect"
	"strings"
	"testing"
)

func ids(evals []Evaluation) []string {
	out := []string{}
	for _, e := range evals {
		out = append(out, e.NodeID)
	}
	return out
}

func TestEligibleFilters(t *testing.T) {
	gpu := map[string]string{"gpu": "yes"}
	cands := []Candidate{
		{ID: "d", Schedulable: true, Online: true},
		{ID: "c", Schedulable: true, Online: true, Labels: gpu},
		{ID: "b", Schedulable: false, Online: true},
		{ID: "a", Schedulable: true, Online: false},
		{ID: "e", Schedulable: true, Online: true, Taints: []Taint{{Key: "dedicated", Value: "db", Effect: EffectNoSchedule}}},
	}
	tests := []struct {
		name string
		req  Request
		want []string
	}{
		{"all schedulable online, untainted, sorted", Request{}, []string{"c", "d"}},
		{"selector", Request{Selector: Selector{"gpu": "yes"}}, []string{"c"}},
		{"toleration admits tainted node", Request{Tolerations: []Toleration{{Key: "dedicated", Operator: "Exists"}}}, []string{"c", "d", "e"}},
		{"exclude", Request{Exclude: "c"}, []string{"d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(Eligible(cands, tt.req)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestEligibleAffinityAndCapacity(t *testing.T) {
	cands := []Candidate{
		{ID: "a", Schedulable: true, Online: true, Capacity: Capacity{CPUMilli: Measured(1000), MemoryBytes: Measured(1 << 30), DiskTotalBytes: Measured(10 << 30), DiskFreeBytes: Measured(10 << 30)}},
		{ID: "b", Schedulable: true, Online: true, Capacity: Capacity{CPUMilli: Measured(4000), MemoryBytes: Measured(8 << 30), DiskTotalBytes: Measured(10 << 30), DiskFreeBytes: Measured(10 << 30)}},
	}
	fit, short := EligibleSplit(cands, Request{Needs: Resources{CPUMilli: 2000}})
	if !reflect.DeepEqual(ids(fit), []string{"b"}) || !reflect.DeepEqual(ids(short), []string{"a"}) {
		t.Fatalf("fit %v short %v", ids(fit), ids(short))
	}
	if !strings.Contains(short[0].Reason, "insufficient capacity") {
		t.Fatalf("reason %q", short[0].Reason)
	}
	// Filtered-out nodes appear in neither list.
	cands = append(cands, Candidate{ID: "z", Online: true})
	fit, short = EligibleSplit(cands, Request{})
	if len(fit) != 2 || len(short) != 0 {
		t.Fatalf("fit %v short %v", ids(fit), ids(short))
	}
}

func TestEligibleMatchesEvaluate(t *testing.T) {
	c := Candidate{ID: "a", Schedulable: true, Online: true}
	req := Request{}
	if got := Eligible([]Candidate{c}, req); len(got) != 1 || got[0].Eligible != Evaluate(c, req).Eligible {
		t.Fatalf("Eligible disagrees with Evaluate: %v", got)
	}
}

func TestEligibleOrdinalPeers(t *testing.T) {
	placed := map[int]string{2: "n3", 0: "n1", 1: "n2", 3: ""}
	if got := OrdinalPeers(placed, 1); !reflect.DeepEqual(got, []string{"n1", "n3"}) {
		t.Fatalf("got %v", got)
	}
	if got := OrdinalPeers(nil, 0); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	// Peers steer the next ordinal away from the nodes the others use.
	cands := []Candidate{{ID: "n1", Schedulable: true, Online: true}, {ID: "n2", Schedulable: true, Online: true}}
	d, err := Spread(cands, Request{Peers: OrdinalPeers(map[int]string{0: "n1"}, 1)})
	if err != nil || d.NodeID == "" {
		t.Fatalf("spread: %v %v", d.NodeID, err)
	}
}
