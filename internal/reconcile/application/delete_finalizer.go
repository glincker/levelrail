package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TeardownStore is the persistence DeleteFinalizer needs. *store.DB satisfies it.
type TeardownStore interface {
	ServiceStore
	AddPendingTeardown(ctx context.Context, name, nodeID string) error
	DeletePendingTeardown(ctx context.Context, name string) error
	RecordPendingTeardownFailure(ctx context.Context, name, cause string) error
	ListPendingTeardowns(ctx context.Context) ([]store.PendingTeardown, error)
}

// RuntimeResolver maps a node ID to the runtime that reaches it.
type RuntimeResolver func(nodeID string) (docker.Runtime, error)

// DeleteFinalizer removes the containers of deleted apps. A tombstone is
// written before desired state is deleted and cleared only after the
// containers are confirmed gone, so an unreachable node or a failed stop is
// retried on every reconcile pass instead of orphaning containers.
type DeleteFinalizer struct {
	store   TeardownStore
	resolve RuntimeResolver
	opts    []Option
	logger  *slog.Logger
}

// NewDeleteFinalizer builds a DeleteFinalizer. opts are applied to the
// throwaway Controller used for container matching (instance ID, prefix).
func NewDeleteFinalizer(s TeardownStore, resolve RuntimeResolver, logger *slog.Logger, opts ...Option) *DeleteFinalizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeleteFinalizer{store: s, resolve: resolve, opts: opts, logger: logger}
}

// Name implements reconcile.Controller.
func (f *DeleteFinalizer) Name() string { return "application/delete-finalizer" }

// Begin writes the tombstone for name. Call before deleting desired state.
func (f *DeleteFinalizer) Begin(ctx context.Context, name, nodeID string) error {
	if err := f.store.AddPendingTeardown(ctx, name, nodeID); err != nil {
		return fmt.Errorf("record teardown for %q: %w", name, err)
	}
	return nil
}

// Finalize tears name's containers down on nodeID and clears its tombstone.
// On failure the tombstone stays and the cause is recorded for retry.
func (f *DeleteFinalizer) Finalize(ctx context.Context, name, nodeID string) error {
	err := f.teardown(ctx, name, nodeID)
	if err != nil {
		if rerr := f.store.RecordPendingTeardownFailure(ctx, name, err.Error()); rerr != nil {
			f.logger.Error("application: record teardown failure", slog.String("error", rerr.Error()), slog.String("name", name))
		}
		return err
	}
	if err := f.store.DeletePendingTeardown(ctx, name); err != nil {
		return fmt.Errorf("clear teardown tombstone for %q: %w", name, err)
	}
	return nil
}

func (f *DeleteFinalizer) teardown(ctx context.Context, name, nodeID string) error {
	runtime, err := f.resolve(nodeID)
	if err != nil {
		return fmt.Errorf("resolve node %q runtime: %w", nodeID, err)
	}
	if err := New(name, f.store, runtime, f.opts...).Teardown(ctx); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}
	return nil
}

// Reconcile implements reconcile.Controller: retries every tombstone.
func (f *DeleteFinalizer) Reconcile(ctx context.Context) (reconcile.Result, error) {
	pending, err := f.store.ListPendingTeardowns(ctx)
	if err != nil {
		return notReady("ListPendingTeardownsFailed", err), fmt.Errorf("application/delete-finalizer: %w", err)
	}
	if len(pending) == 0 {
		return ready("NothingToFinalize"), nil
	}
	var errs []error
	for _, p := range pending {
		if _, err := f.store.GetDesiredService(ctx, p.Name); err == nil {
			// Recreated under the same name: its own controller owns the containers now.
			if derr := f.store.DeletePendingTeardown(ctx, p.Name); derr != nil {
				errs = append(errs, derr)
			}
			continue
		} else if !errors.Is(err, store.ErrServiceNotFound) {
			errs = append(errs, fmt.Errorf("load %q: %w", p.Name, err))
			continue
		}
		tctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		ferr := f.Finalize(tctx, p.Name, p.NodeID)
		cancel()
		if ferr != nil {
			f.logger.Warn("application: teardown of deleted app still pending", slog.String("name", p.Name), slog.String("node_id", p.NodeID), slog.String("error", ferr.Error()))
			errs = append(errs, fmt.Errorf("%s: %w", p.Name, ferr))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return notReady("TeardownPending", err), fmt.Errorf("application/delete-finalizer: %w", err)
	}
	return ready("TeardownsFinalized"), nil
}
