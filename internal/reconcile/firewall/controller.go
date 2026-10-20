// Package firewall implements the reconcile.Controller that converges
// internal/store's firewall_rules table onto the local host's ufw via
// internal/firewall. v1 only manages rules whose NodeID is the local
// node ("" sentinel); a remote-node rule is skipped and logged, not
// silently dropped, pending a future per-node agent RPC.
package firewall

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/firewall"
)

// localNodeID is the sentinel meaning "the control plane's own node".
const localNodeID = ""

// RuleStore is the narrow surface this controller needs from
// internal/store, so tests can fake it without a real database.
// *store.DB satisfies this.
type RuleStore interface {
	ListFirewallRules(ctx context.Context) ([]store.FirewallRule, error)
}

// Syncer is the narrow surface this controller needs from
// internal/firewall.Manager, so tests can fake it without shelling out
// to a real ufw binary. *firewall.Manager satisfies this.
type Syncer interface {
	Sync(ctx context.Context, want []firewall.Rule) (firewall.Result, error)
}

// Controller converges the local host's ufw rules to match every
// firewall_rules row assigned to the control plane's own local node.
type Controller struct {
	store         RuleStore
	syncer        Syncer
	requiredPorts []int
	logger        *slog.Logger
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithLogger overrides the logger used for per-rule skip decisions.
// Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *Controller) { c.logger = logger }
}

// WithRequiredPorts overrides the ports internal/firewall.Validate
// refuses to close, defaulting to firewall.DefaultRequiredPorts. Pass
// the control plane's actually configured ports (management API,
// agent gRPC listener, ingress) rather than relying on the defaults
// whenever any of them were moved off their default address.
func WithRequiredPorts(ports []int) Option {
	return func(c *Controller) { c.requiredPorts = ports }
}

// New builds a Controller.
func New(ruleStore RuleStore, syncer Syncer, opts ...Option) *Controller {
	c := &Controller{store: ruleStore, syncer: syncer, logger: slog.Default(), requiredPorts: firewall.DefaultRequiredPorts}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "firewall" }

// Reconcile implements reconcile.Controller. It lists every desired
// firewall rule, builds the set this controller's local node should
// enforce, re-validates each against the required-ports allowlist as a
// defense-in-depth check (the API layer already refuses an unsafe rule
// at write time, so this should never trigger in practice), and syncs
// the remainder against ufw via Syncer.Sync.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	rules, err := c.store.ListFirewallRules(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("firewall: list firewall rules: %w", err)
	}

	want, skippedRemote, skippedUnsafe := c.wantedRules(ctx, rules)

	result, err := c.syncer.Sync(ctx, want)
	if err != nil {
		return notReady("SyncFailed", err), fmt.Errorf("firewall: sync: %w", err)
	}

	if !result.Installed {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: "UFWNotInstalled",
			Message: "ufw is not installed; if you rely on a different firewall (cloud security groups, firewalld), this is expected and no rules are managed",
		}}}, nil
	}
	if !result.Active {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: "UFWInactive",
			Message: "ufw is installed but inactive; no rules are managed until it's enabled",
		}}}, nil
	}
	if len(result.Errors) > 0 {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: "RuleSyncFailed",
			Message: fmt.Sprintf("%d of %d managed rule(s) failed to sync: %v", len(result.Errors), len(want), result.Errors),
		}}}, nil
	}

	reason := fmt.Sprintf("Synced%dRules", len(want))
	msg := fmt.Sprintf("%d rule(s) synced (%d applied, %d removed this pass)", len(want), result.Applied, result.Removed)
	if skippedRemote > 0 {
		msg += fmt.Sprintf(", %d rule(s) on a remote node skipped (not yet supported)", skippedRemote)
	}
	if skippedUnsafe > 0 {
		msg += fmt.Sprintf(", %d rule(s) skipped as unsafe (would lock out a required platform port)", skippedUnsafe)
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: reason, Message: msg,
	}}}, nil
}

func (c *Controller) wantedRules(ctx context.Context, rules []store.FirewallRule) (want []firewall.Rule, skippedRemote, skippedUnsafe int) {
	for _, r := range rules {
		if r.NodeID != localNodeID {
			skippedRemote++
			c.logger.DebugContext(ctx, "firewall: rule is pinned to a remote node, skipping until per-node firewall management exists",
				slog.String("rule_id", r.ID), slog.String("node_id", r.NodeID))
			continue
		}
		rule := firewall.Rule{
			Port:       r.Port,
			Proto:      r.Protocol,
			SourceCIDR: r.SourceCIDR,
			Action:     firewall.Action(r.Action),
			Owner:      "rule:" + r.ID,
		}
		if err := firewall.Validate(rule, c.requiredPorts); err != nil {
			skippedUnsafe++
			c.logger.ErrorContext(ctx, "firewall: stored rule would lock out a required platform port, skipping",
				slog.String("rule_id", r.ID), slog.String("error", err.Error()))
			continue
		}
		want = append(want, rule)
	}
	return want, skippedRemote, skippedUnsafe
}

func notReady(reason string, err error) reconcile.Result {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: reason, Message: msg,
	}}}
}
