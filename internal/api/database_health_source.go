package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/store"
)

const databaseHealthTimeout = 10 * time.Second

// databaseHealthTarget resolves a running database container, or ErrDatabaseHealthUnavailable.
func (rt *Router) databaseHealthTarget(ctx context.Context, name string) (*store.DesiredDatabase, dbaccess.Exec, string, error) {
	if rt.databases == nil || rt.execRuntime == nil {
		return nil, nil, "", alerting.ErrDatabaseHealthUnavailable
	}
	d, err := rt.databases.GetDesiredDatabase(ctx, name)
	if err != nil {
		if errors.Is(err, store.ErrDatabaseNotFound) {
			return nil, nil, "", alerting.ErrDatabaseHealthUnavailable
		}
		return nil, nil, "", fmt.Errorf("load database %q: %w", name, err)
	}
	state := rt.databaseContainerState(ctx, d)
	if state == nil || !state.Running {
		return nil, nil, "", alerting.ErrDatabaseHealthUnavailable
	}
	runtime, err := rt.execRuntime(d.NodeID)
	if err != nil {
		return nil, nil, "", alerting.ErrDatabaseHealthUnavailable
	}
	return d, runtime, state.ID, nil
}

// DatabaseConnections implements alerting.DatabaseHealthSource.
func (rt *Router) DatabaseConnections(ctx context.Context, name string) (used, limit int, err error) {
	d, ex, id, err := rt.databaseHealthTarget(ctx, name)
	if err != nil {
		return 0, 0, err
	}
	pctx, cancel := context.WithTimeout(ctx, databaseHealthTimeout)
	defer cancel()
	used, limit, err = dbaccess.ReadConnections(pctx, ex, id, d.Engine)
	if errors.Is(err, dbaccess.ErrHealthUnsupported) {
		return 0, 0, alerting.ErrDatabaseHealthUnavailable
	}
	return used, limit, err
}

// DatabaseDiskUsedPercent implements alerting.DatabaseHealthSource.
func (rt *Router) DatabaseDiskUsedPercent(ctx context.Context, name string) (float64, error) {
	d, ex, id, err := rt.databaseHealthTarget(ctx, name)
	if err != nil {
		return 0, err
	}
	pctx, cancel := context.WithTimeout(ctx, databaseHealthTimeout)
	defer cancel()
	pct, err := dbaccess.ReadDiskUsedPercent(pctx, ex, id, d.Engine)
	if errors.Is(err, dbaccess.ErrHealthUnsupported) {
		return 0, alerting.ErrDatabaseHealthUnavailable
	}
	return pct, err
}

// DatabaseUp implements alerting.DatabaseLivenessSource from the container's own state: a running container is up, a stopped or exited one is down.
// A suspended database, a missing runtime or an unreadable container reads as unavailable so the rule goes quiet instead of guessing.
func (rt *Router) DatabaseUp(ctx context.Context, name string) (bool, error) {
	if rt.databases == nil || rt.execRuntime == nil {
		return false, alerting.ErrDatabaseHealthUnavailable
	}
	d, err := rt.databases.GetDesiredDatabase(ctx, name)
	if err != nil {
		if errors.Is(err, store.ErrDatabaseNotFound) {
			return false, alerting.ErrDatabaseHealthUnavailable
		}
		return false, fmt.Errorf("load database %q: %w", name, err)
	}
	if d.Suspended {
		return false, alerting.ErrDatabaseHealthUnavailable
	}
	state := rt.databaseContainerState(ctx, d)
	if state == nil {
		return false, alerting.ErrDatabaseHealthUnavailable
	}
	return state.Running, nil
}
