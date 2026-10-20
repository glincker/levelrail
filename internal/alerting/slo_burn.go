package alerting

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SLO policy env vars. Each tier has _FACTOR, _LONG and _SHORT suffixes, for
// example APP_SLO_PAGE_FAST_FACTOR.
const (
	envSLOWindow      = "APP_SLO_WINDOW"
	envSLOMinRequests = "APP_SLO_MIN_REQUESTS"
	envSLOEvalEvery   = "APP_SLO_EVAL_INTERVAL"
	envSLOTierPrefix  = "APP_SLO_"
)

// Defaults follow the Google SRE workbook multiwindow multi-burn-rate table.
const (
	DefaultSLOWindow      = 30 * 24 * time.Hour
	DefaultSLOMinRequests = 10
	DefaultSLOEvalEvery   = time.Minute
)

// BurnTier fires when the burn rate is at least Factor over both Long and
// Short. The short window makes the alert reset quickly once the burn stops.
type BurnTier struct {
	Name   string        `json:"name"`
	Factor float64       `json:"factor"`
	Long   time.Duration `json:"long"`
	Short  time.Duration `json:"short"`
	Page   bool          `json:"page"`
}

// SLOPolicy is the engine-wide burn-rate configuration.
type SLOPolicy struct {
	Tiers       []BurnTier
	Window      time.Duration
	MinRequests float64
	EvalEvery   time.Duration
}

// DefaultSLOPolicy returns the SRE workbook defaults: 14.4x over 1h and 5m and
// 6x over 6h and 30m page, 3x over 1d and 2h and 1x over 3d and 6h open a ticket.
func DefaultSLOPolicy() SLOPolicy {
	return SLOPolicy{
		Tiers: []BurnTier{
			{Name: "page_fast", Factor: 14.4, Long: time.Hour, Short: 5 * time.Minute, Page: true},
			{Name: "page_slow", Factor: 6, Long: 6 * time.Hour, Short: 30 * time.Minute, Page: true},
			{Name: "ticket_fast", Factor: 3, Long: 24 * time.Hour, Short: 2 * time.Hour},
			{Name: "ticket_slow", Factor: 1, Long: 72 * time.Hour, Short: 6 * time.Hour},
		},
		Window:      DefaultSLOWindow,
		MinRequests: DefaultSLOMinRequests,
		EvalEvery:   DefaultSLOEvalEvery,
	}
}

// SLOPolicyFromEnv applies APP_SLO_* overrides to the defaults. A malformed or
// non-positive value keeps the default.
func SLOPolicyFromEnv() SLOPolicy {
	p := DefaultSLOPolicy()
	dur := func(key string, dst *time.Duration) {
		if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
			*dst = d
		}
	}
	dur(envSLOWindow, &p.Window)
	dur(envSLOEvalEvery, &p.EvalEvery)
	if n, err := strconv.ParseFloat(os.Getenv(envSLOMinRequests), 64); err == nil && n >= 0 {
		p.MinRequests = n
	}
	for i := range p.Tiers {
		t := &p.Tiers[i]
		prefix := envSLOTierPrefix + strings.ToUpper(t.Name)
		if f, err := strconv.ParseFloat(os.Getenv(prefix+"_FACTOR"), 64); err == nil && f > 0 {
			t.Factor = f
		}
		dur(prefix+"_LONG", &t.Long)
		dur(prefix+"_SHORT", &t.Short)
	}
	return p
}

// BurnWindowStat is the observed burn over one window.
type BurnWindowStat struct {
	Window   time.Duration `json:"window"`
	Requests float64       `json:"requests"`
	Bad      float64       `json:"bad"`
	Burn     float64       `json:"burn_rate"`
}

// TierStatus is one tier's evaluation. Effective is the lower of the long and
// short burn rates, the value compared against Factor.
type TierStatus struct {
	BurnTier
	LongBurn  float64 `json:"long_burn"`
	ShortBurn float64 `json:"short_burn"`
	Effective float64 `json:"effective_burn"`
	Firing    bool    `json:"firing"`
}

// SLOStatus is the full evaluation of an SLO at one instant.
type SLOStatus struct {
	Config          SLOConfig        `json:"config"`
	HasTraffic      bool             `json:"has_traffic"`
	BudgetRemaining float64          `json:"budget_remaining"`
	BudgetRequests  float64          `json:"budget_window_requests"`
	Windows         []BurnWindowStat `json:"windows"`
	Tiers           []TierStatus     `json:"tiers"`
	Firing          bool             `json:"firing"`
	Page            bool             `json:"page"`
	MaxBurn         float64          `json:"max_burn"`
}

// ComputeSLOStatus queries every distinct window once and evaluates all tiers.
// Partial windows (an app newer than the window) are judged on the traffic that
// exists, and a tier whose long window saw fewer than MinRequests cannot fire.
func ComputeSLOStatus(ctx context.Context, src MetricsSource, app string, cfg SLOConfig, p SLOPolicy, now time.Time) (SLOStatus, error) {
	st := SLOStatus{Config: cfg, BudgetRemaining: 1}
	seen := map[time.Duration]BurnWindowStat{}
	need := []time.Duration{p.Window}
	for _, t := range p.Tiers {
		need = append(need, t.Long, t.Short)
	}
	for _, w := range need {
		if _, ok := seen[w]; ok {
			continue
		}
		c, err := CountRequests(ctx, src, app, now.Add(-w), now, cfg)
		if err != nil {
			return st, err
		}
		seen[w] = BurnWindowStat{Window: w, Requests: c.Total, Bad: c.Bad, Burn: BurnRate(c.Bad, c.Total, cfg.Target)}
	}
	for w, s := range seen {
		if w != p.Window {
			st.Windows = append(st.Windows, s)
		}
	}
	sort.Slice(st.Windows, func(i, j int) bool { return st.Windows[i].Window < st.Windows[j].Window })

	budgetWin := seen[p.Window]
	st.HasTraffic = budgetWin.Requests > 0
	st.BudgetRequests = budgetWin.Requests
	st.BudgetRemaining = BudgetRemaining(budgetWin.Bad, budgetWin.Requests, cfg.Target)

	for _, t := range p.Tiers {
		long, short := seen[t.Long], seen[t.Short]
		ts := TierStatus{BurnTier: t, LongBurn: long.Burn, ShortBurn: short.Burn, Effective: min(long.Burn, short.Burn)}
		ts.Firing = long.Requests >= p.MinRequests && long.Requests > 0 && ts.Effective >= t.Factor
		st.Tiers = append(st.Tiers, ts)
		st.MaxBurn = max(st.MaxBurn, ts.Effective)
		if ts.Firing {
			st.Firing = true
			st.Page = st.Page || t.Page
		}
	}
	return st, nil
}

// Notice is the one-line explanation of the firing tier that burns fastest.
func (s SLOStatus) Notice() string {
	var top *TierStatus
	for i := range s.Tiers {
		if s.Tiers[i].Firing && (top == nil || s.Tiers[i].Factor > top.Factor) {
			top = &s.Tiers[i]
		}
	}
	if top == nil {
		return ""
	}
	kind := "ticket"
	if top.Page {
		kind = "page"
	}
	what := fmt.Sprintf("%.4g%% availability", s.Config.Target)
	if s.Config.Objective == SLOLatency {
		what = fmt.Sprintf("%.4g%% under %gms", s.Config.Target, s.Config.LatencyMs)
	}
	return fmt.Sprintf("SLO %s: burning error budget %.1fx over %s and %s (%s, threshold %gx), %.0f%% of the budget left",
		what, top.Effective, shortDuration(top.Long), shortDuration(top.Short), kind, top.Factor, s.BudgetRemaining*100)
}

func shortDuration(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return d.String()
}

// EvaluateSLOBurn runs one KindSLOBurn rule. It returns the rule's next state
// and, while firing, the notice for the notification. A rule with no SLO
// config never fires.
func EvaluateSLOBurn(ctx context.Context, src MetricsSource, r Rule, p SLOPolicy, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	if r.SLO == nil {
		return advanceState(next, r, false, 0, now), "", nil
	}
	app, ok := domainHealthAppName(r.ResourceID)
	if !ok {
		return next, "", fmt.Errorf("alerting: slo_burn rule %q: resource %q is not an app", r.ID, r.ResourceID)
	}
	st, err := ComputeSLOStatus(ctx, src, app, *r.SLO, p, now)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}
	v := st.MaxBurn
	next.LastValue = &v
	next = advanceState(next, r, st.Firing, r.ForDuration, now)
	if !next.Firing {
		return next, "", nil
	}
	if st.Page {
		next.Severity = SeverityCritical
	} else {
		next.Severity = SeverityWarning
	}
	return next, st.Notice(), nil
}
