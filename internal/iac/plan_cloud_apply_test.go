package iac

import (
	"context"
	"strconv"
	"testing"
)

type countingPlanner struct{ n *int }

func (c countingPlanner) PlanCloud(_ context.Context, r CloudResource) (CloudDiff, error) {
	*c.n++
	if r.Kind == KindCloudAccount {
		return CloudDiff{Action: "noop"}, nil
	}
	return CloudDiff{Action: "create", Type: "t", PlanHash: "engine-" + strconv.Itoa(*c.n)}, nil
}

func TestCloudPlanPinnedKeepsHashWithoutReplanning(t *testing.T) {
	res := cloudPlanSetup(t)
	n := 0
	SetCloudPlanner(countingPlanner{n: &n})
	first := PlanFor(newState(), res, Options{})
	again := PlanFor(newState(), res, Options{})
	if first.Hash == again.Hash {
		t.Fatal("a fresh engine plan must change the plan hash")
	}
	pins := map[string]Change{}
	for _, c := range first.Changes {
		pins[c.Key()] = c
	}
	before := n
	pinned := PlanFor(newState(), res, Options{Cloud: &CloudApproval{Pins: pins}})
	if pinned.Hash != first.Hash || n != before {
		t.Fatalf("pinned hash %s want %s, planner calls %d", pinned.Hash, first.Hash, n-before)
	}
	delete(pins, resourceKey(KindCloudBucket, "", "backups"))
	if p := PlanFor(newState(), res, Options{Cloud: &CloudApproval{Pins: pins}}); p.Summary.Error != 1 {
		t.Fatalf("unpinned document must fail: %+v", p.Summary)
	}
}
