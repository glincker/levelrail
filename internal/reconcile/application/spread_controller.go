package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/store"
)

// SpreadSub is one node's share of a spread service, a *Controller in practice.
type SpreadSub interface {
	Reconcile(ctx context.Context) (reconcile.Result, error)
	Teardown(ctx context.Context) error
}

// SpreadDeps wires a SpreadController without tying it to boot code.
type SpreadDeps struct {
	Store ServiceStore
	// Plan returns the node of each replica index, or nil to keep every replica on NodeID.
	Plan func(ctx context.Context, svc *store.DesiredService) ([]string, string)
	// Commit persists ReplicaNodes (nil clears it) once the new nodes are proven ready.
	Commit func(ctx context.Context, name string, nodes []string) error
	// Build makes a node's controller: nil indexes means every replica, empty means cleanup only.
	// rec must be installed last so it wins over any real deploy recorder.
	Build func(node string, indexes []int, rec DeployRecorder) SpreadSub
	// Recorder receives one deploy record per rollout, however many nodes deployed.
	Recorder DeployRecorder
}

// SpreadController runs one service as per-node sub controllers under a single status.
type SpreadController struct {
	name string
	deps SpreadDeps

	mu        sync.Mutex
	lastNodes []string
}

// NewSpreadController builds the wrapper for serviceName.
func NewSpreadController(serviceName string, deps SpreadDeps) *SpreadController {
	return &SpreadController{name: serviceName, deps: deps}
}

// Name keeps the plain application identity so status readers need no change.
func (s *SpreadController) Name() string { return ControllerName(s.name) }

type spreadGroup struct {
	node    string
	indexes []int
}

// deployCapture counts deploy events from the subs so the wrapper records one.
type deployCapture struct {
	mu sync.Mutex
	n  int
}

func (d *deployCapture) RecordDeploy(context.Context, string, time.Time) error {
	d.mu.Lock()
	d.n++
	d.mu.Unlock()
	return nil
}

func (d *deployCapture) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.n
}

// Reconcile implements reconcile.Controller.
func (s *SpreadController) Reconcile(ctx context.Context) (reconcile.Result, error) {
	svc, err := s.deps.Store.GetDesiredService(ctx, s.name)
	if errors.Is(err, store.ErrServiceNotFound) {
		return unknownResult("NoDesiredState"), nil
	}
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("application/%s: get desired service: %w", s.name, err)
	}

	plan := s.plan(ctx, svc)
	groups := groupByNode(plan)
	stale := staleNodes(svc, plan)
	s.remember(groups, stale)

	capture := &deployCapture{}
	var results []nodeResult
	var errs []error
	for _, g := range groups {
		indexes := g.indexes
		if len(groups) == 1 && len(indexes) == len(plan) {
			indexes = nil
		}
		res, rerr := s.deps.Build(g.node, indexes, capture).Reconcile(ctx)
		results = append(results, nodeResult{node: g.node, res: res})
		if rerr != nil {
			errs = append(errs, rerr)
		}
	}
	agg := aggregateSpread(results)
	if !isReady(agg) {
		return agg, errors.Join(errs...)
	}

	// New nodes are proven, so ingress may now route to them and the old ones can drain.
	if want := committedNodes(svc, plan); !slices.Equal(want, svc.ReplicaNodes) && s.deps.Commit != nil {
		if cerr := s.deps.Commit(ctx, s.name, want); cerr != nil {
			return notReady("SpreadCommitFailed", cerr), fmt.Errorf("application/%s: commit replica nodes: %w", s.name, cerr)
		}
	}
	for _, node := range stale {
		if _, cerr := s.deps.Build(node, []int{}, capture).Reconcile(ctx); cerr != nil {
			errs = append(errs, cerr)
		}
	}
	if capture.count() > 0 && s.deps.Recorder != nil {
		if rerr := s.deps.Recorder.RecordDeploy(ctx, s.name, time.Now()); rerr != nil {
			errs = append(errs, fmt.Errorf("record deploy metric: %w", rerr))
		}
	}
	return agg, errors.Join(errs...)
}

// Teardown removes the service from every node it was last seen on.
func (s *SpreadController) Teardown(ctx context.Context) error {
	s.mu.Lock()
	nodes := slices.Clone(s.lastNodes)
	s.mu.Unlock()
	var errs []error
	for _, node := range nodes {
		if err := s.deps.Build(node, []int{}, &deployCapture{}).Teardown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *SpreadController) remember(groups []spreadGroup, stale []string) {
	var nodes []string
	for _, g := range groups {
		nodes = append(nodes, g.node)
	}
	nodes = append(nodes, stale...)
	s.mu.Lock()
	s.lastNodes = nodes
	s.mu.Unlock()
}

// plan returns the node of every replica index, falling back to the home node.
func (s *SpreadController) plan(ctx context.Context, svc *store.DesiredService) []string {
	replicas := max(svc.Replicas, store.DefaultReplicas)
	var nodes []string
	switch {
	case s.deps.Plan != nil:
		nodes, _ = s.deps.Plan(ctx, svc)
	case len(svc.ReplicaNodes) > 0:
		nodes = svc.ReplicaNodes
	}
	if len(nodes) != replicas {
		nodes = make([]string, replicas)
		for i := range nodes {
			nodes[i] = svc.NodeID
		}
	}
	return nodes
}

func groupByNode(plan []string) []spreadGroup {
	var groups []spreadGroup
	for i, node := range plan {
		at := slices.IndexFunc(groups, func(g spreadGroup) bool { return g.node == node })
		if at < 0 {
			groups = append(groups, spreadGroup{node: node})
			at = len(groups) - 1
		}
		groups[at].indexes = append(groups[at].indexes, i)
	}
	return groups
}

// staleNodes lists nodes that held replicas before and have none in the new plan.
func staleNodes(svc *store.DesiredService, plan []string) []string {
	var out []string
	for _, node := range append([]string{svc.NodeID}, svc.ReplicaNodes...) {
		if !slices.Contains(plan, node) && !slices.Contains(out, node) {
			out = append(out, node)
		}
	}
	return out
}

// committedNodes is the ReplicaNodes value for plan: nil when everything sits on the home node.
func committedNodes(svc *store.DesiredService, plan []string) []string {
	if !slices.ContainsFunc(plan, func(n string) bool { return n != svc.NodeID }) {
		return nil
	}
	return plan
}

type nodeResult struct {
	node string
	res  reconcile.Result
}

func nodeLabel(node string) string {
	if node == "" {
		return "control plane"
	}
	return node
}

// aggregateSpread folds per-node results into one Ready condition: ready only when every node is.
func aggregateSpread(results []nodeResult) reconcile.Result {
	var out reconcile.Result
	reason := ""
	for _, nr := range results {
		if nr.res.RequeueAfter > 0 && (out.RequeueAfter == 0 || nr.res.RequeueAfter < out.RequeueAfter) {
			out.RequeueAfter, out.KeyOnly = nr.res.RequeueAfter, nr.res.KeyOnly
		}
		for _, c := range nr.res.Conditions {
			if c.Type != reconcile.ConditionTypeReady {
				out.Conditions = append(out.Conditions, c)
				continue
			}
			if c.Status != reconcile.ConditionTrue {
				first := reconcile.Condition{Type: c.Type, Status: c.Status, Reason: c.Reason, Message: nodeLabel(nr.node) + ": " + c.Message}
				if !hasNotReady(out) {
					out.Conditions = append([]reconcile.Condition{first}, out.Conditions...)
				}
				continue
			}
			if reason == "" || c.Reason == "Deployed" {
				reason = c.Reason
			}
		}
	}
	if !hasNotReady(out) {
		ready := reconcile.Condition{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: reason}
		out.Conditions = append([]reconcile.Condition{ready}, out.Conditions...)
	}
	return out
}

func hasNotReady(r reconcile.Result) bool {
	for _, c := range r.Conditions {
		if c.Type == reconcile.ConditionTypeReady && c.Status != reconcile.ConditionTrue {
			return true
		}
	}
	return false
}
