package application

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/store"
)

type spreadHarness struct {
	rts      map[string]*fakeRuntime
	builds   map[string][]int
	fenced   map[string]int
	commits  [][]string
	deploys  int
	mu       sync.Mutex
	svc      *store.DesiredService
	planNode []string
}

func newSpreadHarness(svc *store.DesiredService, plan []string) *spreadHarness {
	return &spreadHarness{rts: map[string]*fakeRuntime{}, builds: map[string][]int{}, fenced: map[string]int{}, svc: svc, planNode: plan}
}

func (h *spreadHarness) RecordDeploy(context.Context, string, time.Time) error {
	h.deploys++
	return nil
}

func (h *spreadHarness) controller() *SpreadController {
	return NewSpreadController(h.svc.Name, SpreadDeps{
		Store: &fakeStore{svc: h.svc},
		Plan:  func(context.Context, *store.DesiredService) ([]string, string) { return h.planNode, "" },
		Commit: func(_ context.Context, _ string, nodes []string) error {
			h.commits = append(h.commits, nodes)
			return nil
		},
		Recorder: h,
		Build: func(node string, indexes []int, rec DeployRecorder) SpreadSub {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.builds[node] = indexes
			rt, ok := h.rts[node]
			if !ok {
				rt = newFakeRuntime(0)
				h.rts[node] = rt
			}
			opts := []Option{WithFence(func(ctx context.Context, _ string) context.Context {
				h.mu.Lock()
				h.fenced[node]++
				h.mu.Unlock()
				return ctx
			}), WithDeployRecorder(rec)}
			if indexes != nil {
				opts = append(opts, WithReplicaIndexes(indexes))
			}
			return New(h.svc.Name, &fakeStore{svc: h.svc}, rt, opts...)
		},
	})
}

var errBoom = errors.New("boom")

func spreadSvc(replicas int) *store.DesiredService {
	return &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: store.DefaultDeployStrategy, Replicas: replicas, NodeID: ""}
}

func TestSpreadControllersSplitReplicasByNode(t *testing.T) {
	h := newSpreadHarness(spreadSvc(3), []string{"", "n1", ""})
	if _, err := h.controller().Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if !slices.Equal(h.builds[""], []int{0, 2}) || !slices.Equal(h.builds["n1"], []int{1}) {
		t.Fatalf("builds = %v, want control plane [0 2] and n1 [1]", h.builds)
	}
	if got := h.rts[""].names(); !slices.Contains(got, replicaContainerName("web", "img:v1", "", 2)) || len(got) != 2 {
		t.Errorf("control plane containers = %v, want replicas 0 and 2", got)
	}
	if got := h.rts["n1"].names(); !slices.Equal(got, []string{replicaContainerName("web", "img:v1", "", 1)}) {
		t.Errorf("n1 containers = %v, want replica 1 only", got)
	}
	if len(h.commits) != 1 || !slices.Equal(h.commits[0], []string{"", "n1", ""}) {
		t.Errorf("commits = %v, want one commit of the plan", h.commits)
	}
	if h.deploys != 1 {
		t.Errorf("deploy records = %d, want 1", h.deploys)
	}
}

func TestSpreadStatusAggregatesAcrossNodes(t *testing.T) {
	h := newSpreadHarness(spreadSvc(2), []string{"", "n1"})
	c := h.controller()
	h.rts["n1"] = newFakeRuntime(0)
	h.rts["n1"].createErr = errBoom
	res, _ := c.Reconcile(context.Background())
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Fatalf("condition = %+v, want False CreateFailed from n1", cond)
	}
	if len(h.commits) != 0 || h.deploys != 0 {
		t.Errorf("commits=%v deploys=%d, want none while a node is not ready", h.commits, h.deploys)
	}
	h.rts["n1"].createErr = nil
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want True Deployed", cond)
	}
}

func TestSpreadEmptyReplicaNodesUnchanged(t *testing.T) {
	h := newSpreadHarness(spreadSvc(2), nil)
	if _, err := h.controller().Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	indexes, built := h.builds[""]
	if !built || indexes != nil || len(h.builds) != 1 {
		t.Fatalf("builds = %v, want one unfiltered controller on the home node", h.builds)
	}
	if len(h.commits) != 0 {
		t.Errorf("commits = %v, want none for an unspread service", h.commits)
	}
	if h.rts[""].count() != 2 {
		t.Errorf("containers = %d, want 2", h.rts[""].count())
	}
}

func TestSpreadFenceCarriedPerNode(t *testing.T) {
	h := newSpreadHarness(spreadSvc(2), []string{"", "n1"})
	if _, err := h.controller().Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, node := range []string{"", "n1"} {
		if h.fenced[node] == 0 {
			t.Errorf("node %q ran without the fence stamp, counts = %v", node, h.fenced)
		}
	}
}
