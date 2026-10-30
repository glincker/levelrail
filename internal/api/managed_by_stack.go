package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

type stackOwnerLister interface {
	ListStackOwnerNames(ctx context.Context) (map[string]string, error)
}

// stackOwners is one request's snapshot of which stack owns each resource.
// A nil value (flag off, store lacking support, query failure) answers "".
type stackOwners map[string]string

// loadStackOwners runs the single ledger query for a request, and only when
// the iac feature is on. Failures degrade to no badge rather than an error.
func (rt *Router) loadStackOwners(ctx context.Context) stackOwners {
	if !experimental.Enabled(experimental.IaC) {
		return nil
	}
	l, ok := rt.apps.(stackOwnerLister)
	if !ok {
		return nil
	}
	m, err := l.ListStackOwnerNames(ctx)
	if err != nil {
		rt.logger.Warn("api: stack owner lookup failed", slog.String("error", err.Error()))
		return nil
	}
	return m
}

// of returns the owning stack's name for an iac kind and ledger key, or "".
func (o stackOwners) of(kind, key string) string {
	return o[store.StackOwnerKey(kind, key)]
}
