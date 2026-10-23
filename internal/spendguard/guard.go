// Package spendguard refuses new deploys of an app whose spend cap or cost
// budget is in block_deploys mode and has reached 100% this month. It reads
// the state the cap controllers already keep, so it is deterministic and
// needs no usage query on the deploy path.
package spendguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

// ErrDeploysBlocked is wrapped by every refusal.
var ErrDeploysBlocked = errors.New("deploys are blocked by a spend cap")

// Store is the persistence the guard reads; *store.DB satisfies it.
type Store interface {
	GetSpendCap(ctx context.Context, app string) (spendcap.Cap, error)
	GetSpendCapState(ctx context.Context, app string) (spendcap.State, bool, error)
	ListCostBudgets(ctx context.Context) ([]costbudget.Budget, error)
	GetCostBudgetState(ctx context.Context, id string) (costbudget.State, bool, error)
	ActiveSpendOverride(ctx context.Context, app string, now time.Time) (spendcap.Override, bool, error)
}

// Guard decides whether a deploy may proceed.
type Guard struct {
	Store Store
	// ProjectOf maps an app to its project id; nil or "" means no project.
	ProjectOf func(ctx context.Context, app string) (string, error)
	Logger    *slog.Logger
	Now       func() time.Time
}

// Reason is one cap or budget holding an app at its limit.
type Reason struct {
	Source string  `json:"source"`
	Pct    float64 `json:"pct"`
}

// Status is the guard's verdict for one app.
type Status struct {
	Blocked  bool               `json:"blocked"`
	Allowed  bool               `json:"allowed"`
	Reasons  []Reason           `json:"reasons"`
	Override *spendcap.Override `json:"override,omitempty"`
}

// BlockedError is returned when a deploy is refused.
type BlockedError struct {
	App     string
	Reasons []Reason
}

func (e *BlockedError) Error() string {
	r := e.Reasons[0]
	return fmt.Sprintf("deploys of %q are blocked: %s is at %.0f%% of its monthly limit. Raise the limit, wait for the month to roll over, or have a root user override the guard from the app's Spend card", e.App, r.Source, r.Pct)
}

// Unwrap lets callers match ErrDeploysBlocked.
func (e *BlockedError) Unwrap() error { return ErrDeploysBlocked }

func (g *Guard) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Guard) appReason(ctx context.Context, app, period string) ([]Reason, error) {
	cp, err := g.Store.GetSpendCap(ctx, app)
	if errors.Is(err, spendcap.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("spendguard: cap: %w", err)
	}
	if cp.GuardMode != spendcap.GuardBlockDeploys {
		return nil, nil
	}
	s, ok, err := g.Store.GetSpendCapState(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("spendguard: cap state: %w", err)
	}
	if ok && s.Period == period && s.Pct >= 100 {
		return []Reason{{Source: "the app's spend cap", Pct: s.Pct}}, nil
	}
	return nil, nil
}

func (g *Guard) budgetReasons(ctx context.Context, app, period string) ([]Reason, error) {
	budgets, err := g.Store.ListCostBudgets(ctx)
	if err != nil {
		return nil, fmt.Errorf("spendguard: budgets: %w", err)
	}
	var out []Reason
	project, projectLoaded := "", false
	for _, b := range budgets {
		if b.GuardMode != spendcap.GuardBlockDeploys {
			continue
		}
		covers := b.Scope == costbudget.ScopeApp && b.ScopeValue == app
		if b.Scope == costbudget.ScopeProject {
			if !projectLoaded {
				projectLoaded = true
				if g.ProjectOf != nil {
					if project, err = g.ProjectOf(ctx, app); err != nil {
						return nil, fmt.Errorf("spendguard: project of %q: %w", app, err)
					}
				}
			}
			covers = project != "" && project == b.ScopeValue
		}
		if !covers {
			continue
		}
		s, ok, err := g.Store.GetCostBudgetState(ctx, b.ID)
		if err != nil {
			return nil, fmt.Errorf("spendguard: budget state: %w", err)
		}
		if ok && s.Period == period && s.Pct >= 100 {
			out = append(out, Reason{Source: fmt.Sprintf("budget %q", b.Name), Pct: s.Pct})
		}
	}
	return out, nil
}

// Check returns the guard's verdict for app at the current time.
func (g *Guard) Check(ctx context.Context, app string) (Status, error) {
	now := g.now()
	period := spendcap.Period(now)
	st := Status{Reasons: []Reason{}, Allowed: true}
	appR, err := g.appReason(ctx, app, period)
	if err != nil {
		return st, err
	}
	budR, err := g.budgetReasons(ctx, app, period)
	if err != nil {
		return st, err
	}
	st.Reasons = append(append(st.Reasons, appR...), budR...)
	st.Blocked = len(st.Reasons) > 0
	if !st.Blocked {
		return st, nil
	}
	o, ok, err := g.Store.ActiveSpendOverride(ctx, app, now)
	if err != nil {
		return st, fmt.Errorf("spendguard: override: %w", err)
	}
	if ok {
		st.Override = &o
		return st, nil
	}
	st.Allowed = false
	return st, nil
}

// BeforeDeploy refuses a new deploy of a blocked app. A failure to read the
// guard state lets the deploy through, because a cost guard must never stop
// an app from shipping a fix on a storage hiccup.
func (g *Guard) BeforeDeploy(ctx context.Context, app string) error {
	st, err := g.Check(ctx, app)
	if err != nil {
		if g.Logger != nil {
			g.Logger.WarnContext(ctx, "spendguard: check failed, allowing deploy", slog.String("app", app), slog.String("error", err.Error()))
		}
		return nil
	}
	if !st.Allowed {
		return &BlockedError{App: app, Reasons: st.Reasons}
	}
	return nil
}
