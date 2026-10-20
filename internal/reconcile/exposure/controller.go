// Package exposure converges the host's DOCKER-USER chain onto the stored
// exposure restrictions, so they survive a reboot or Docker restart.
package exposure

import (
	"context"
	"fmt"
	"log/slog"

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
}

// New builds a Controller.
func New(s RestrictionStore, syncer Syncer, logger *slog.Logger) *Controller {
	if logger == nil {
		logger = slog.Default()
	}
	return &Controller{store: s, syncer: syncer, logger: logger}
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
		c.logger.WarnContext(ctx, "exposure: sync reported errors", slog.Int("errors", len(res.Errors)), slog.String("first", res.Errors[0]))
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
