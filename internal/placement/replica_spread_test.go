package placement

import (
	"errors"
	"testing"
)

func TestPlanReplicasSkipsTaintedNode(t *testing.T) {
	a, b, c := capNode("a", 0), capNode("b", 0), capNode("c", 0)
	c.Taints = []Taint{{Key: "gpu", Effect: EffectNoSchedule}}
	plan, err := PlanReplicas([]Candidate{a, b, c}, Request{}, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, n := range plan {
		counts[n]++
	}
	if counts["c"] != 0 || counts["a"]+counts["b"] != 3 || counts["a"] == counts["b"] {
		t.Fatalf("want a 2/1 split over a and b, got %v", counts)
	}
}

func TestPlanReplicasKeepsEligibleCurrent(t *testing.T) {
	plan, err := PlanReplicas([]Candidate{capNode("a", 0), capNode("b", 0)}, Request{}, 3, []string{"a", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if plan[0] != "a" || plan[1] != "a" || plan[2] != "b" {
		t.Fatalf("plan = %v, want the first two kept on a and the new one on b", plan)
	}
}

func TestPlanReplicasNoEligibleNode(t *testing.T) {
	a := capNode("a", 0)
	a.Online = false
	if _, err := PlanReplicas([]Candidate{a}, Request{}, 2, nil); !errors.Is(err, ErrNoEligibleNode) {
		t.Fatalf("err = %v, want ErrNoEligibleNode", err)
	}
}

func TestSpreadBlockRefusesVolumes(t *testing.T) {
	if SpreadBlock(1, 0) == "" || SpreadBlock(0, 1) == "" {
		t.Fatal("volumes and bind mounts must block a spread")
	}
	if SpreadBlock(0, 0) != "" {
		t.Fatal("a stateless app may spread")
	}
}
