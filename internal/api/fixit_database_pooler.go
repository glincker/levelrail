package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseConnectionsFixPercent reads the usage that counts as high; it defaults to the alert rule default.
func databaseConnectionsFixPercent() float64 {
	return envFloat("APP_FIXIT_DB_CONNECTIONS_PERCENT", alerting.DefaultDatabaseConnectionsPercent)
}

// poolerCandidates lists running Postgres databases without a pooler whose connection use is at or above the alert threshold.
func (rt *Router) poolerCandidates(ctx context.Context) ([]fixit.PoolerCandidate, error) {
	if rt.databases == nil {
		return nil, nil
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	at := databaseConnectionsFixPercent()
	var out []fixit.PoolerCandidate
	for _, d := range dbs {
		if d.Engine != store.EnginePostgres || d.PoolerEnabled || d.Suspended {
			continue
		}
		used, limit, err := rt.DatabaseConnections(ctx, d.Name)
		if errors.Is(err, alerting.ErrDatabaseHealthUnavailable) || err == nil && limit <= 0 {
			continue
		}
		if err != nil {
			return nil, err
		}
		if pct := float64(used) * 100 / float64(limit); pct >= at {
			out = append(out, fixit.PoolerCandidate{Name: d.Name, Detail: fmt.Sprintf("%d of %d connections in use", used, limit)})
		}
	}
	return out, nil
}

// enablePoolerForFix follows the enable-pooler endpoint's path: flag it on, restart linked apps, wake the reconciler.
func (rt *Router) enablePoolerForFix(ctx context.Context, name string) error {
	ps, ok := rt.databases.(databasePoolerStore)
	if !ok {
		return fixit.ErrCannotFixHere
	}
	if err := ps.SetDatabasePooler(ctx, name, true); err != nil {
		return err
	}
	rt.afterPoolerChange(ctx, name)
	return nil
}

func (rt *Router) databasePoolerFix() fixit.Fix {
	return fixit.DatabasePoolerFix(fixit.DatabasePoolerDeps{
		Candidates: rt.poolerCandidates,
		Enable:     rt.enablePoolerForFix,
		Blocked: func() string {
			if database.PoolerImage() == "" {
				return "no pooler image is configured: set the databases.pooler_image setting to a pinned tag or digest"
			}
			return ""
		},
	})
}
