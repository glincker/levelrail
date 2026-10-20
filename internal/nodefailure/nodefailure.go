// Package nodefailure is the opt-in, deterministic policy for nodes that stay
// unreachable: it reschedules stateless apps, never moves anything holding
// node-local data, and fences stale copies when the node returns.
package nodefailure

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Resource is one app or database placed on a node.
type Resource struct {
	Kind    string
	Name    string
	Volumes []string
}

// Config holds the control plane defaults a per-node policy overrides.
type Config struct {
	Mode         string
	Grace        time.Duration
	RecoveryHold time.Duration
	Interval     time.Duration
}

// ConfigFromEnv reads APP_NODE_FAILURE_MODE (off, notify, auto), APP_NODE_FAILURE_GRACE,
// APP_NODE_FAILURE_RECOVERY_HOLD and APP_NODE_FAILURE_INTERVAL. The default mode is off.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	cfg := Config{Mode: store.NodeFailureModeOff, Grace: 5 * time.Minute, RecoveryHold: 2 * time.Minute, Interval: 15 * time.Second}
	if v, ok := lookup("APP_NODE_FAILURE_MODE"); ok && store.IsNodeFailureMode(v) {
		cfg.Mode = v
	}
	set := func(key string, dst *time.Duration) {
		if v, ok := lookup(key); ok {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				*dst = d
			}
		}
	}
	set("APP_NODE_FAILURE_GRACE", &cfg.Grace)
	set("APP_NODE_FAILURE_RECOVERY_HOLD", &cfg.RecoveryHold)
	set("APP_NODE_FAILURE_INTERVAL", &cfg.Interval)
	return cfg
}

// Store is the persistence surface the controller needs; *store.DB satisfies it.
type Store interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
	GetNode(ctx context.Context, id string) (*store.Node, error)
	SetNodeSchedulable(ctx context.Context, id string, schedulable bool) error
	GetNodeFailurePolicy(ctx context.Context, nodeID string) (store.NodeFailurePolicy, error)
	SetNodeFailurePolicy(ctx context.Context, p store.NodeFailurePolicy) error
	GetNodeFailureState(ctx context.Context, nodeID string) (*store.NodeFailureState, error)
	ListNodeFailureStates(ctx context.Context) (map[string]store.NodeFailureState, error)
	SaveNodeFailureState(ctx context.Context, s store.NodeFailureState) error
	DeleteNodeFailureState(ctx context.Context, nodeID string) error
}

// Executor acts on the platform. Placement, teardown and audit stay with the caller.
type Executor interface {
	IsLocal(nodeID string) bool
	Resources(ctx context.Context, nodeID string) ([]Resource, error)
	NodeOf(ctx context.Context, kind, name string) (string, error)
	// PickTarget chooses a healthy node for res; commit false must not record anything.
	PickTarget(ctx context.Context, res Resource, fromNodeID string, commit bool) (string, error)
	Place(ctx context.Context, kind, name, nodeID string) error
	// Fence stops the stale copy of a resource on nodeID.
	Fence(ctx context.Context, kind, name, nodeID string) error
	Audit(ctx context.Context, action, detail string)
}

// Errors returned to operators.
var (
	ErrNodeHealthy  = errors.New("nodefailure: node is reachable")
	ErrNoFailure    = errors.New("nodefailure: node has no failure in progress")
	ErrNoSuchAction = errors.New("nodefailure: no such resource on this node's failure list")
)

// Controller converges every node's failure state each tick; it is level-triggered and idempotent.
type Controller struct {
	store  Store
	exec   Executor
	cfg    Config
	logger *slog.Logger
	now    func() time.Time
}

// Option configures a Controller.
type Option func(*Controller)

// WithClock overrides the clock for tests.
func WithClock(now func() time.Time) Option { return func(c *Controller) { c.now = now } }

// New builds a Controller.
func New(s Store, e Executor, cfg Config, logger *slog.Logger, opts ...Option) *Controller {
	c := &Controller{store: s, exec: e, cfg: cfg, logger: logger, now: time.Now}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Run ticks until ctx is done.
func (c *Controller) Run(ctx context.Context) {
	t := time.NewTicker(c.cfg.Interval)
	defer t.Stop()
	for {
		c.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick evaluates every node once.
func (c *Controller) Tick(ctx context.Context) {
	nodes, err := c.store.ListNodes(ctx)
	if err != nil {
		c.logger.Error("nodefailure: list nodes failed", slog.String("error", err.Error()))
		return
	}
	for i := range nodes {
		if c.exec.IsLocal(nodes[i].ID) || nodes[i].Status == store.NodeStatusPending {
			continue
		}
		if err := c.process(ctx, nodes[i]); err != nil {
			c.logger.Error("nodefailure: process node failed", slog.String("node_id", nodes[i].ID), slog.String("error", err.Error()))
		}
	}
}

// Classify says what a failure of the node holding r means for r.
func Classify(r Resource) (class, reason string) {
	switch {
	case r.Kind == store.NodeMoveKindDatabase:
		return store.NodeFailureClassDecision, "database data lives on the failed node"
	case len(r.Volumes) > 0:
		return store.NodeFailureClassDecision, "node-local volumes: " + strconv.Itoa(len(r.Volumes)) + " on the failed node"
	}
	return store.NodeFailureClassReschedule, "stateless, can start on a healthy node"
}

func (c *Controller) policy(ctx context.Context, nodeID string) (mode string, grace time.Duration, err error) {
	p, err := c.store.GetNodeFailurePolicy(ctx, nodeID)
	if err != nil {
		return "", 0, err
	}
	mode, grace = c.cfg.Mode, c.cfg.Grace
	if p.Mode != "" {
		mode = p.Mode
	}
	if p.GraceSeconds > 0 {
		grace = time.Duration(p.GraceSeconds) * time.Second
	}
	return mode, grace, nil
}

func silentFor(n store.Node, now time.Time) (time.Duration, bool) {
	if n.LastSeenAt == nil {
		return 0, false
	}
	return now.Sub(*n.LastSeenAt), true
}

func (c *Controller) unreachable(n store.Node, grace time.Duration) bool {
	d, seen := silentFor(n, c.now())
	return !seen || d > grace
}
