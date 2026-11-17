package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/placement"
)

func TestRolloutGroupSelectionAndBatchOrder(t *testing.T) {
	mk := func(id, name string, l map[string]string) nodeLabelSet {
		return nodeLabelSet{Node: apiclient.NodeResource{ID: id, Name: name}, Labels: l}
	}
	sets := []nodeLabelSet{
		mk("n3", "pi-c", map[string]string{"fleet": "edge", "site": "a"}),
		mk("n1", "pi-a", map[string]string{"fleet": "edge"}),
		mk("n2", "pi-b", map[string]string{"fleet": "lab"}),
		mk("n4", "pi-d", map[string]string{"fleet": "edge"}),
	}
	apps := []apiclient.AppResource{
		{Name: "sensor-a", NodeID: "n1"}, {Name: "sensor", NodeID: "n3"},
		{Name: "sensor-b", NodeID: "n2"}, {Name: "sensor-d", NodeID: "n4"}, {Name: "other", NodeID: "n1"},
	}
	batches := planRollout(sets, apps, "sensor", placement.Selector{"fleet": "edge"}, 2)
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 1 {
		t.Fatalf("batches = %+v", batches)
	}
	if batches[0][0].Node != "pi-a" || batches[0][1].Node != "pi-c" || batches[1][0].Node != "pi-d" {
		t.Fatalf("order = %+v", batches)
	}
	g := groupsFromLabels(sets)
	if g[0].Selector != "fleet=edge" || len(g[0].Nodes) != 3 {
		t.Fatalf("groups = %+v", g)
	}
}
