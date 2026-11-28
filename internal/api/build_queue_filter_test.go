package api

import "testing"

func TestFilterBuildQueueKeepsVisibleApps(t *testing.T) {
	res := buildQueueResource{
		Running: []buildQueueRunning{{App: "a"}, {App: "b"}},
		Queued:  []buildQueueQueued{{App: "b"}, {App: "a"}},
	}
	filterBuildQueue(&res, []string{"a"})
	if len(res.Running) != 1 || res.Running[0].App != "a" || len(res.Queued) != 1 || res.Queued[0].App != "a" {
		t.Fatalf("filtered = %+v", res)
	}
	all := buildQueueResource{Running: []buildQueueRunning{{App: "a"}, {App: "b"}}}
	filterBuildQueue(&all, nil)
	if len(all.Running) != 2 {
		t.Fatalf("nil visibility must keep all, got %+v", all)
	}
}
