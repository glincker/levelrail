package nodefailure

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	nodes  map[string]*store.Node
	policy store.NodeFailurePolicy
	state  map[string]*store.NodeFailureState
}

func (f *fakeStore) ListNodes(context.Context) ([]store.Node, error) {
	var out []store.Node
	for _, n := range f.nodes {
		out = append(out, *n)
	}
	return out, nil
}
func (f *fakeStore) GetNode(_ context.Context, id string) (*store.Node, error) {
	n := f.nodes[id]
	return n, nil
}
func (f *fakeStore) SetNodeSchedulable(_ context.Context, id string, v bool) error {
	f.nodes[id].Schedulable = v
	return nil
}
func (f *fakeStore) GetNodeFailurePolicy(context.Context, string) (store.NodeFailurePolicy, error) {
	return f.policy, nil
}
func (f *fakeStore) SetNodeFailurePolicy(context.Context, store.NodeFailurePolicy) error { return nil }
func (f *fakeStore) ListNodeFailureStates(context.Context) (map[string]store.NodeFailureState, error) {
	return nil, nil
}
func (f *fakeStore) GetNodeFailureState(_ context.Context, id string) (*store.NodeFailureState, error) {
	if s := f.state[id]; s != nil {
		c := *s
		c.Actions = append([]store.NodeFailureAction(nil), s.Actions...)
		return &c, nil
	}
	return nil, nil
}
func (f *fakeStore) SaveNodeFailureState(_ context.Context, s store.NodeFailureState) error {
	f.state[s.NodeID] = &s
	return nil
}
func (f *fakeStore) DeleteNodeFailureState(_ context.Context, id string) error {
	delete(f.state, id)
	return nil
}

type fakeExec struct {
	placed   map[string]string
	volumes  map[string][]string
	failNext bool
	fenced   []string
	audits   int
}

func (e *fakeExec) IsLocal(string) bool { return false }
func (e *fakeExec) Resources(_ context.Context, nodeID string) ([]Resource, error) {
	var out []Resource
	for name, node := range e.placed {
		if node == nodeID {
			out = append(out, Resource{Kind: store.NodeMoveKindApp, Name: name, Volumes: e.volumes[name]})
		}
	}
	return out, nil
}
func (e *fakeExec) NodeOf(_ context.Context, _, name string) (string, error) {
	return e.placed[name], nil
}
func (e *fakeExec) PickTarget(context.Context, Resource, string, bool) (string, error) {
	return "n2", nil
}
func (e *fakeExec) Place(_ context.Context, _, name, node string) error {
	if e.failNext {
		e.failNext = false
		return errors.New("boom")
	}
	e.placed[name] = node
	return nil
}
func (e *fakeExec) Fence(_ context.Context, _, name, node string) error {
	e.fenced = append(e.fenced, name+"@"+node)
	return nil
}
func (e *fakeExec) Audit(context.Context, string, string) { e.audits++ }

func setup(lastSeen time.Time, mode string) (*Controller, *fakeStore, *fakeExec, *time.Time) {
	now := lastSeen
	fs := &fakeStore{
		nodes:  map[string]*store.Node{"n1": {ID: "n1", Status: store.NodeStatusOnline, Schedulable: true, LastSeenAt: &lastSeen}},
		policy: store.NodeFailurePolicy{Mode: mode},
		state:  map[string]*store.NodeFailureState{},
	}
	fe := &fakeExec{placed: map[string]string{"web": "n1", "db-app": "n1"}, volumes: map[string][]string{"db-app": {"data"}}}
	cfg := Config{Mode: store.NodeFailureModeOff, Grace: time.Minute, RecoveryHold: time.Minute, Interval: time.Second}
	c := New(fs, fe, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), WithClock(func() time.Time { return now }))
	return c, fs, fe, &now
}

func TestReschedulesStatelessOnlyAndRetriesHalfSuccess(t *testing.T) {
	seen := time.Unix(1000, 0)
	c, fs, fe, now := setup(seen, store.NodeFailureModeAuto)
	ctx := context.Background()

	*now = seen.Add(30 * time.Second)
	c.Tick(ctx)
	if fs.state["n1"] != nil {
		t.Fatal("state created inside the grace period")
	}

	*now = seen.Add(2 * time.Minute)
	fe.failNext = true
	c.Tick(ctx)
	if fe.placed["web"] != "n1" {
		t.Fatal("failed placement must leave the app where it was")
	}
	c.Tick(ctx)
	if fe.placed["web"] != "n2" {
		t.Fatalf("retry after half-success should move the stateless app, got %q", fe.placed["web"])
	}
	if fe.placed["db-app"] != "n1" {
		t.Fatal("app with node-local volumes must never move automatically")
	}
	for _, a := range fs.state["n1"].Actions {
		if a.Name == "db-app" && a.Class != store.NodeFailureClassDecision {
			t.Fatalf("db-app class %s", a.Class)
		}
	}
}

func TestFencingOnReturnAndFlapHysteresis(t *testing.T) {
	seen := time.Unix(1000, 0)
	c, fs, fe, now := setup(seen, store.NodeFailureModeAuto)
	ctx := context.Background()

	*now = seen.Add(2 * time.Minute)
	c.Tick(ctx)
	if fe.placed["web"] != "n2" {
		t.Fatal("expected reschedule")
	}

	back := *now
	fs.nodes["n1"].LastSeenAt = &back
	c.Tick(ctx)
	if len(fe.fenced) != 1 || fe.fenced[0] != "web@n1" {
		t.Fatalf("stale copy must be fenced when the node returns, got %v", fe.fenced)
	}
	if fs.state["n1"].Phase != store.NodeFailurePhaseRecovering {
		t.Fatalf("phase %s", fs.state["n1"].Phase)
	}

	*now = back.Add(2 * time.Minute)
	c.Tick(ctx)
	if fs.state["n1"].Phase != store.NodeFailurePhaseUnreachable {
		t.Fatal("a node that drops again during recovery goes back to unreachable")
	}
	if fe.placed["web"] != "n2" || len(fe.fenced) != 1 {
		t.Fatal("flapping must not move or fence anything twice")
	}

	again := *now
	fs.nodes["n1"].LastSeenAt = &again
	c.Tick(ctx)
	*now = again.Add(2 * time.Minute)
	fs.nodes["n1"].LastSeenAt = &[]time.Time{*now}[0]
	c.Tick(ctx)
	if fs.state["n1"] != nil || !fs.nodes["n1"].Schedulable {
		t.Fatal("state should clear and the node be uncordoned after the recovery hold")
	}
}
