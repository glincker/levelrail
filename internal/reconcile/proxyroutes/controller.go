// Package proxyroutes is the reconcile.Controller that keeps route files in
// an external Traefik's dynamic directory in step with the routed domains.
package proxyroutes

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// Name is the controller's stable name.
const Name = "proxy-routes"

// Condition reasons.
const (
	ReasonDisabled  = "Disabled"
	ReasonConverged = "Converged"
	ReasonFailed    = "WriteFailed"
	ReasonStore     = "StoreError"
)

// Syncer is the part of *proxyroutes.Syncer the controller drives.
type Syncer interface {
	Sync(ctx context.Context) (proxyroutes.Report, error)
}

// Controller runs one Sync per reconcile pass. Every pass re-derives the
// whole directory from the store, so it is level-triggered and safe to
// interrupt.
type Controller struct {
	syncer Syncer
}

// New returns a controller over syncer, which should be shared with the API
// so passes on the same directory are serialised.
func New(syncer Syncer) *Controller { return &Controller{syncer: syncer} }

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return Name }

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	rep, err := c.syncer.Sync(ctx)
	switch {
	case err != nil && len(rep.Failed) > 0:
		return result(reconcile.ConditionFalse, ReasonFailed, err.Error()), fmt.Errorf("proxy routes: %w", err)
	case err != nil:
		return result(reconcile.ConditionFalse, ReasonStore, err.Error()), fmt.Errorf("proxy routes: %w", err)
	case !rep.Enabled:
		return result(reconcile.ConditionTrue, ReasonDisabled, "managed proxy routes are off"), nil
	}
	return result(reconcile.ConditionTrue, ReasonConverged,
		fmt.Sprintf("%d written, %d removed", len(rep.Written), len(rep.Removed))), nil
}

func result(status reconcile.ConditionStatus, reason, msg string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: reconcile.ConditionTypeReady, Status: status, Reason: reason, Message: msg,
	}}}
}
