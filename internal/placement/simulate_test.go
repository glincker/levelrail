package placement

import (
	"reflect"
	"testing"
)

func sized(c Candidate, memMiB int64) Candidate {
	c.Capacity = Capacity{
		CPUMilli: Measured(4000), MemoryBytes: Measured(memMiB << 20),
		DiskTotalBytes: Measured(100 << 30), DiskFreeBytes: Measured(80 << 30),
	}
	return c
}

func TestSimulateSpreadsAndDoesNotMutate(t *testing.T) {
	cands := []Candidate{sized(fleetNode("a1", "a", 0), 1024), sized(fleetNode("b1", "b", 0), 1024)}
	before := append([]Candidate(nil), cands...)
	res := Simulate(cands, Request{Needs: Resources{MemoryBytes: 512 << 20}}, 5)
	if res.Unplaced != 1 || len(res.Placed) != 5 {
		t.Fatalf("placed %d unplaced %d, want 5 and 1: %+v", len(res.Placed), res.Unplaced, res.Placed)
	}
	if res.Placed[4].Unplaced == "" {
		t.Fatalf("fifth app should be refused for memory: %+v", res.Placed[4])
	}
	if res.Placed[0].Zone == res.Placed[1].Zone {
		t.Fatalf("first two apps share a zone: %+v", res.Placed)
	}
	if !reflect.DeepEqual(cands, before) {
		t.Fatal("simulation changed the caller's candidates")
	}
}

func TestFailNodesReschedulesDeterministically(t *testing.T) {
	cands := []Candidate{
		sized(fleetNode("a1", "a", 2), 4096), sized(fleetNode("b1", "b", 0), 4096), sized(fleetNode("c1", "c", 0), 4096),
	}
	work := []Workload{
		{Name: "web", NodeID: "a1", Needs: Resources{MemoryBytes: 256 << 20}},
		{Name: "api", NodeID: "a1", Needs: Resources{MemoryBytes: 512 << 20}},
	}
	dead := func(c Candidate) bool { return ZoneOf(c.Labels) == "a" }
	first := FailNodes(cands, work, dead)
	second := FailNodes(cands, work, dead)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same fleet gave different answers")
	}
	if first.Unplaced != 0 || len(first.Placed) != 2 {
		t.Fatalf("want both rescheduled: %+v", first)
	}
	if first.Placed[0].Name != "api" {
		t.Fatalf("largest workload goes first, got %s", first.Placed[0].Name)
	}
	for _, p := range first.Placed {
		if p.Zone == "a" || p.NodeID == "" {
			t.Fatalf("%s landed in the failed zone: %+v", p.Name, p)
		}
	}
}

func TestFailNodesReportsUnplaceable(t *testing.T) {
	cands := []Candidate{sized(fleetNode("a1", "a", 1), 1024), sized(fleetNode("b1", "b", 0), 1024)}
	work := []Workload{{Name: "big", NodeID: "a1", Needs: Resources{MemoryBytes: 2048 << 20}}}
	res := FailNodes(cands, work, func(c Candidate) bool { return c.ID == "a1" })
	if res.Unplaced != 1 || res.Placed[0].Unplaced == "" {
		t.Fatalf("want the workload reported unplaceable: %+v", res)
	}
}
