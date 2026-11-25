package api

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/dbbranch"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const branchExpiryRuleID = "database-branch-expiry"

// WithBranchBackends enables the live branch clone with the engine dump and restore runners.
func WithBranchBackends(d dbbranch.Dumper, r dbbranch.Restorer) Option {
	return func(rt *Router) { rt.branchCloner = &routerBranchCloner{rt: rt, dumper: d, restorer: r} }
}

// routerBranchCloner builds the clone worker per run so it always sees the router's current stores.
type routerBranchCloner struct {
	rt       *Router
	dumper   dbbranch.Dumper
	restorer dbbranch.Restorer
}

func (c *routerBranchCloner) Run(ctx context.Context, b store.DatabaseBranch, engine string) error {
	cs, ok := c.rt.databases.(dbbranch.CloneStore)
	if !ok {
		return fmt.Errorf("api: branch clone: store has no branch support")
	}
	w := &dbbranch.Cloner{
		Store: cs, Dumper: c.dumper, Restorer: c.restorer,
		WaitReady: c.rt.waitDatabaseReady,
		Teardown:  c.rt.TeardownBranchDatabase,
		Container: databaseContainerName,
		Timeout: func(ctx context.Context) time.Duration {
			return c.rt.runtimeSettings.Duration(ctx, runtimesettings.KeyBranchesCloneTimeout)
		},
		Log: c.rt.logger,
	}
	return w.Run(ctx, b, engine)
}

// waitDatabaseReady polls the database reconciler's conditions until Ready; the context bounds the wait.
func (rt *Router) waitDatabaseReady(ctx context.Context, name string) error {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		conds, err := rt.deploys.GetConditions(ctx, databaseControllerName(name))
		if err != nil {
			return err
		}
		for _, c := range conds {
			if c.Type == reconcile.ConditionTypeReady && c.Status == reconcile.ConditionTrue {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// StartBranchExpiry runs the expiry job. Settings are read live on every tick.
func (rt *Router) StartBranchExpiry(ctx context.Context) {
	es, ok := rt.databases.(dbbranch.ExpiryStore)
	if !ok {
		return
	}
	job := &dbbranch.ExpiryJob{
		Store: es,
		Policy: func(ctx context.Context) dbbranch.ExpiryPolicy {
			s := rt.runtimeSettings
			interval := s.Duration(ctx, runtimesettings.KeyBranchesExpiryInterval)
			return dbbranch.ExpiryPolicy{
				Enabled: s.Bool(ctx, runtimesettings.KeyBranchesEnabled), Interval: interval,
				Warning: s.Duration(ctx, runtimesettings.KeyBranchesExpiryWarning), FailedRetention: s.Duration(ctx, runtimesettings.KeyBranchesFailedRetention),
				Retry: 2 * interval, RequestedTimeout: s.Duration(ctx, runtimesettings.KeyBranchesCloneTimeout),
			}
		},
		Delete: func(ctx context.Context, b store.DatabaseBranch) error {
			if err := rt.TeardownBranchDatabase(ctx, b.DatabaseName); err != nil {
				return err
			}
			rt.AuditBranchExpired(ctx, b)
			return nil
		},
		Warn: rt.warnBranchExpiry,
		Log:  rt.logger,
	}
	go job.Run(ctx)
}

// warnBranchExpiry writes an inbox entry through the same history path the DNS watch uses.
func (rt *Router) warnBranchExpiry(ctx context.Context, b store.DatabaseBranch, expiresAt time.Time) {
	h, ok := rt.inbox.(dnsWatchHistory)
	if !ok {
		return
	}
	_ = h.RecordHistory(ctx, alerting.HistoryEntry{
		RuleID: branchExpiryRuleID, RuleName: "Database branch expiring", RuleKind: branchExpiryRuleID,
		ResourceID: "database-branch:" + b.DatabaseName + "@" + expiresAt.UTC().Format(time.RFC3339),
		Severity:   alerting.SeverityInfo, Event: alerting.EventResolved, Outcome: alerting.OutcomeSent,
		Detail: "Branch " + b.DatabaseName + " is deleted at " + expiresAt.UTC().Format(time.RFC3339) + ". Extend or protect it to keep it.",
	})
}
