package api

import (
	"context"
	"testing"

	"github.com/glincker/levelrail/internal/janitor"
	"github.com/glincker/levelrail/internal/nodecapacity"
)

type fakeBudgetAgent struct {
	calls []bool
	prev  nodecapacity.Preview
}

func (f *fakeBudgetAgent) NodeCleanup(_ context.Context, dryRun bool, _ []string) (nodecapacity.Result, error) {
	f.calls = append(f.calls, dryRun)
	return nodecapacity.Result{Preview: f.prev}, nil
}

func TestNodeBudgetPreviewListsOldestAndFreesNothing(t *testing.T) {
	gb := int64(1) << 30
	items := []janitor.Item{}
	for _, n := range []string{"img-oldest", "img-b", "img-c", "img-d", "img-e", "img-f"} {
		items = append(items, janitor.Item{Name: n, SizeBytes: 2 * gb, Action: janitor.ActionKept})
	}
	agent := &fakeBudgetAgent{prev: nodecapacity.Preview{Classes: []nodecapacity.Class{{Name: nodecapacity.ClassRollbackImages, Items: items}}}}
	_, plan, err := previewNodeBudget(context.Background(), agent, nil, 10*gb)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Over() || plan.OverBytes != 2*gb || plan.UnusedBytes != 12*gb {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Oldest) != 1 || plan.Oldest[0].Name != "img-oldest" {
		t.Fatalf("oldest = %+v", plan.Oldest)
	}
	if plan.WouldFreeBytes != 0 {
		t.Fatalf("kept rollback images must not count as freed: %d", plan.WouldFreeBytes)
	}
	for _, dry := range agent.calls {
		if !dry {
			t.Fatal("the preview sent a removing request")
		}
	}
}
