package build

import (
	"strings"
	"testing"
)

func TestChooseBuildNodeSkipsLowDiskRunner(t *testing.T) {
	nodes := []NodeInfo{
		{ID: "a", Name: "a", AcceptsBuildWorkloads: true, Online: true, DiskFreeBytes: 512 << 20},
		{ID: "b", Name: "b", AcceptsBuildWorkloads: true, Online: true, DiskFreeBytes: 8 << 30},
	}
	p := Policy{Strategy: StrategyFirst, MinDiskFreeBytes: 2 << 30}
	if got := ChooseBuildNode(nodes, Query{}, p); got.Node != "b" {
		t.Fatalf("node = %q, want b (a is low on disk)", got.Node)
	}
	if why := nodes[0].SkipReason(p); !strings.Contains(why, "disk low") {
		t.Fatalf("SkipReason = %q, want a disk low reason", why)
	}
	if why := nodes[1].SkipReason(p); why != "" {
		t.Fatalf("SkipReason for a healthy node = %q, want empty", why)
	}
	if got := ChooseBuildNode(nodes[:1], Query{}, Policy{Strategy: StrategyFirst}); got.Node != "a" {
		t.Fatalf("with the gate off node = %q, want a", got.Node)
	}
}
