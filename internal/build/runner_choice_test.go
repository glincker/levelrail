package build

import (
	"strings"
	"testing"
	"time"
)

func runnerNode(id string, used int) NodeInfo {
	return NodeInfo{ID: id, Name: id, AcceptsBuildWorkloads: true, Online: true, SlotsUsed: used, SlotsTotal: 2, Pressure: PressureOK}
}

func TestRunnerChooseLeastLoaded(t *testing.T) {
	p := Policy{Strategy: StrategyLeastLoaded, PressureHold: true}
	d := ChooseBuildNode([]NodeInfo{runnerNode("a", 2), runnerNode("b", 0)}, Query{}, p)
	if d.Node != "b" || d.Reason == "" {
		t.Fatalf("want b with a reason, got %+v", d)
	}
}

func TestRunnerChoosePressureAndDraining(t *testing.T) {
	hot := runnerNode("a", 0)
	hot.Pressure = PressureCritical
	drain := runnerNode("b", 0)
	drain.Draining = true
	p := Policy{Strategy: StrategyLeastLoaded, PressureHold: true}
	if d := ChooseBuildNode([]NodeInfo{hot, drain}, Query{}, p); d.Node != "" || !strings.HasPrefix(d.Reason, "control plane") {
		t.Fatalf("want local fallback, got %+v", d)
	}
	for _, s := range []Strategy{StrategyFirst, StrategyLeastLoaded, StrategySticky} {
		if d := ChooseBuildNode([]NodeInfo{drain}, Query{}, Policy{Strategy: s}); d.Node != "" {
			t.Fatalf("%s chose a draining node", s)
		}
	}
}

func TestRunnerChooseArchMismatch(t *testing.T) {
	arm := runnerNode("a", 0)
	arm.Arch = "linux/arm64"
	amd := runnerNode("b", 1)
	amd.Arch = "linux/amd64"
	d := ChooseBuildNode([]NodeInfo{arm, amd}, Query{TargetArch: "linux/amd64"}, Policy{Strategy: StrategyLeastLoaded})
	if d.Node != "b" {
		t.Fatalf("want b, got %+v", d)
	}
	if d := ChooseBuildNode([]NodeInfo{arm}, Query{TargetArch: "linux/amd64"}, Policy{Strategy: StrategyLeastLoaded}); d.Node != "" {
		t.Fatalf("want fallback, got %+v", d)
	}
}

func TestRunnerChooseStickyPinnedLabels(t *testing.T) {
	a, b := runnerNode("a", 0), runnerNode("b", 1)
	if d := ChooseBuildNode([]NodeInfo{a, b}, Query{LastNode: "b"}, Policy{Strategy: StrategySticky}); d.Node != "b" {
		t.Fatalf("sticky: %+v", d)
	}
	if d := ChooseBuildNode([]NodeInfo{a, b}, Query{Pinned: "b"}, Policy{Strategy: StrategyLeastLoaded}); d.Node != "b" {
		t.Fatalf("pinned: %+v", d)
	}
	b.Online = false
	if d := ChooseBuildNode([]NodeInfo{a, b}, Query{Pinned: "b"}, Policy{Strategy: StrategyLeastLoaded}); d.Node != "" {
		t.Fatalf("pinned offline must fall back local: %+v", d)
	}
	a.Labels = map[string]string{"gpu": "true"}
	if d := ChooseBuildNode([]NodeInfo{a, runnerNode("c", 0)}, Query{Labels: map[string]string{"gpu": ""}}, Policy{Strategy: StrategyLeastLoaded}); d.Node != "a" {
		t.Fatalf("labels: %+v", d)
	}
}

func TestRunnerChooseFirstKeepsOrderAndStale(t *testing.T) {
	a, b := runnerNode("a", 2), runnerNode("b", 0)
	if d := ChooseBuildNode([]NodeInfo{b, a}, Query{}, Policy{Strategy: StrategyFirst}); d.Node != "a" {
		t.Fatalf("first: %+v", d)
	}
	now := time.Now()
	a.LastSeen = now.Add(-time.Hour)
	if d := ChooseBuildNode([]NodeInfo{a}, Query{}, Policy{Strategy: StrategyLeastLoaded, StaleAfter: time.Minute, Now: now}); d.Node != "" {
		t.Fatalf("stale node chosen: %+v", d)
	}
}

func TestRunnerChooseModes(t *testing.T) {
	nodes := []NodeInfo{runnerNode("a", 0), runnerNode("b", 1)}
	p := Policy{Strategy: StrategyLeastLoaded}
	if d := ChooseBuildNode(nodes, Query{Mode: ModeControlPlane, Preferred: "a", Pinned: "b"}, p); d.Node != "" || !strings.HasPrefix(d.Reason, "control plane") {
		t.Fatalf("control_plane must stay local, got %+v", d)
	}
	if d := ChooseBuildNode(nodes, Query{Mode: ModeAuto}, p); d.Node != "a" {
		t.Fatalf("auto should pick the freest runner, got %+v", d)
	}
	if d := ChooseBuildNode(nodes, Query{Mode: ModeNode, Pinned: "b"}, p); d.Node != "b" {
		t.Fatalf("pinned node mode should pick b, got %+v", d)
	}
}
