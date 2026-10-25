// Package exposure converges the host's DOCKER-USER chain onto the stored
// exposure restrictions, so they survive a reboot or Docker restart.
package exposure

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RestrictionStore is the narrow store surface this controller needs.
type RestrictionStore interface {
	ListExposureRestrictions(ctx context.Context) ([]store.ExposureRestriction, error)
}

// Syncer is the narrow surface of exposure.Manager this controller needs.
type Syncer interface {
	Sync(ctx context.Context, want []exposure.Restriction) exposure.SyncResult
}

// Controller re-asserts every stored restriction on each pass.
type Controller struct {
	store  RestrictionStore
	syncer Syncer
	logger *slog.Logger

	podman func(context.Context) bool
}

// Option tunes a Controller.
type Option func(*Controller)

// WithPodmanCheck tells the controller whether the engine is Podman, whose
// netavark uses nftables so the iptables based check cannot apply.
func WithPodmanCheck(f func(context.Context) bool) Option {
	return func(c *Controller) { c.podman = f }
}

// seenCauses is process wide on purpose: it holds across controller rebuilds and
// retriggered passes, so one permanent cause logs once per process lifetime.
var seenCauses sync.Map

var variablePart = regexp.MustCompile(`[0-9]+|"[^"]*"|\(.*\)`)

// normalizeCause strips numbers, quoted values and parentheses so two passes
// of the same condition share one key.
func normalizeCause(msg string) string { return variablePart.ReplaceAllString(msg, "#") }

// resetSeenCauses clears the process-wide set (tests only).
func resetSeenCauses() { seenCauses = sync.Map{} }

// New builds a Controller.
func New(s RestrictionStore, syncer Syncer, logger *slog.Logger, opts ...Option) *Controller {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Controller{store: s, syncer: syncer, logger: logger}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "exposure" }

// Reconcile implements reconcile.Controller. With no stored restriction and
// no leftover tagged rule it does nothing, so a host that never opted in is untouched.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	rows, err := c.store.ListExposureRestrictions(ctx)
	if err != nil {
		return notReady("StoreError", err.Error()), fmt.Errorf("exposure: list restrictions: %w", err)
	}
	want := make([]exposure.Restriction, 0, len(rows))
	for _, r := range rows {
		want = append(want, exposure.Restriction{Port: r.Port, Protocol: r.Protocol, Allow: r.Allow})
	}
	res := c.syncer.Sync(ctx, want)
	if len(res.Errors) > 0 {
		c.logSyncErrors(ctx, res.Errors)
		if len(want) == 0 {
			return reconcile.Result{Conditions: []reconcile.Condition{{
				Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: "ChainUnreadable", Message: res.Errors[0],
			}}}, nil
		}
		return notReady("RestrictionSyncFailed", fmt.Sprintf("%d error(s), first: %s", len(res.Errors), res.Errors[0])), nil
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: fmt.Sprintf("Synced%dRestrictions", len(want)),
		Message: fmt.Sprintf("%d restriction(s) in place (%d applied, %d removed this pass)", len(want), res.Applied, res.Removed),
	}}}, nil
}

func notReady(reason, msg string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: reason, Message: msg,
	}}}
}

// logSyncErrors logs each distinct cause once at WARN (INFO under Podman for
// the iptables cause), then at DEBUG for every later pass.
func (c *Controller) logSyncErrors(ctx context.Context, errs []string) {
	first := errs[0]
	_, seen := seenCauses.LoadOrStore(normalizeCause(first), struct{}{})
	if seen {
		c.logger.DebugContext(ctx, "exposure: sync reported errors", slog.Int("errors", len(errs)), slog.String("first", first))
		return
	}
	if c.podman != nil && c.podman(ctx) && strings.Contains(first, "iptables is not installed") {
		c.logger.InfoContext(ctx, "exposure: iptables based exposure check skipped, this engine uses nftables")
		return
	}
	c.logger.WarnContext(ctx, "exposure: sync reported errors", slog.Int("errors", len(errs)), slog.String("first", first))
}
