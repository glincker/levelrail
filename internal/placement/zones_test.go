package placement

import (
	"fmt"
	"testing"
)

func fleetNode(id, zone string, load int) Candidate {
	labels := map[string]string{LabelRegion: "eu", LabelProvider: "hetzner"}
	if zone != "" {
		labels[LabelZone] = zone
	}
	return Candidate{ID: id, Labels: labels, Schedulable: true, Online: true, Load: load}
}

func zoneFleet() []Candidate {
	return []Candidate{
		fleetNode("a1", "a", 0), fleetNode("a2", "a", 0),
		fleetNode("b1", "b", 0), fleetNode("c1", "c", 0),
	}
}

func placeReplicas(t *testing.T, cands []Candidate, n int) []string {
	t.Helper()
	var placed []string
	for i := 0; i < n; i++ {
		d, err := Spread(cands, Request{Peers: placed})
		if err != nil {
			t.Fatalf("replica %d: %v", i, err)
		}
		placed = append(placed, d.NodeID)
		for j := range cands {
			if cands[j].ID == d.NodeID {
				cands[j].Load++
			}
		}
	}
	return placed
}

func zonesOf(cands []Candidate, ids []string) map[string]int {
	byID := map[string]string{}
	for _, c := range cands {
		byID[c.ID] = ZoneOf(c.Labels)
	}
	out := map[string]int{}
	for _, id := range ids {
		out[byID[id]]++
	}
	return out
}

func TestSpreadReplicasAcrossZonesByDefault(t *testing.T) {
	cands := zoneFleet()
	placed := placeReplicas(t, cands, 3)
	got := zonesOf(cands, placed)
	if len(got) != 3 {
		t.Fatalf("3 replicas landed in zones %v, want one per zone", got)
	}
	placed = append(placed, placeReplicas(t, cands, 3)...)
	for zone, n := range zonesOf(cands, placed) {
		if n != 2 {
			t.Fatalf("zone %s has %d replicas of 6, want 2 each", zone, n)
		}
	}
}

func TestSpreadWithoutZoneLabelsKeepsHeadroomOrder(t *testing.T) {
	cands := []Candidate{
		{ID: "a", Schedulable: true, Online: true, Load: 2},
		{ID: "b", Schedulable: true, Online: true, Load: 0},
	}
	d, err := Spread(cands, Request{Peers: []string{"b"}})
	if err != nil || d.NodeID != "b" {
		t.Fatalf("got %q, %v; want b (no zones, so load decides)", d.NodeID, err)
	}
}

func TestZoneLossReplacesReplicasInSurvivingZones(t *testing.T) {
	cands := zoneFleet()
	placed := placeReplicas(t, cands, 3)
	for i := range cands {
		if ZoneOf(cands[i].Labels) == "a" {
			cands[i].Online = false
		}
	}
	var survivors, lost []string
	for _, id := range placed {
		if id == "a1" || id == "a2" {
			lost = append(lost, id)
		} else {
			survivors = append(survivors, id)
		}
	}
	if len(lost) != 1 {
		t.Fatalf("expected one replica in zone a, got %v", lost)
	}
	d, err := Spread(cands, Request{Peers: survivors})
	if err != nil {
		t.Fatalf("replacement after zone loss: %v", err)
	}
	if z := ZoneOf(labelsOf(cands, d.NodeID)); z == "a" || z == "" {
		t.Fatalf("replacement landed in zone %q, want a surviving zone", z)
	}
}

func TestZoneAwareDrainPrefersOtherZone(t *testing.T) {
	cands := []Candidate{
		fleetNode("a1", "a", 3), fleetNode("a2", "a", 0),
		fleetNode("b1", "b", 1),
	}
	d, err := Spread(cands, Request{Exclude: "a1", Peers: []string{"a2"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.NodeID != "b1" {
		t.Fatalf("drain target %s, want b1 (zone b holds no peer)", d.NodeID)
	}
}

func TestZonesSummary(t *testing.T) {
	got := Zones(zoneFleet())
	if len(got) != 3 || got[0].Zone != "a" || len(got[0].NodeIDs) != 2 || got[0].Region != "eu" {
		t.Fatalf("Zones() = %+v", got)
	}
	if fmt.Sprint(got[0].Providers) != "[hetzner]" {
		t.Fatalf("providers = %v", got[0].Providers)
	}
}

func labelsOf(cands []Candidate, id string) map[string]string {
	for _, c := range cands {
		if c.ID == id {
			return c.Labels
		}
	}
	return nil
}
