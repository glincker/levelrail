package docker

import "testing"

func TestMergeNetworkLabelsOwnLabelsWin(t *testing.T) {
	got := mergeNetworkLabels(map[string]string{"inst": "me"}, map[string]string{"inst": "spoof", "extra": "1"})
	if got["inst"] != "me" || got["extra"] != "1" {
		t.Fatalf("merge = %v, want own instance label kept and extra added", got)
	}
	if got := mergeNetworkLabels(nil, map[string]string{"inst": "cp"}); got["inst"] != "cp" {
		t.Fatalf("unscoped client (agent) merge = %v, want the control plane's label applied", got)
	}
	if got := mergeNetworkLabels(nil, nil); got != nil {
		t.Fatalf("no labels = %v, want nil", got)
	}
}
