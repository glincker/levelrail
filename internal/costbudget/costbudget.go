// Package costbudget holds named monthly budgets scoped to a project, tag or
// app, and the level-triggered state machine that decides which alert
// thresholds fired and whether the 100 percent action is in force. It is pure
// arithmetic: usage, persistence and effects live with the callers.
package costbudget

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

// Scope kinds a budget can cover.
const (
	ScopeProject = "project"
	ScopeTag     = "tag"
	ScopeApp     = "app"
)

// MaxThresholds bounds the alert levels of one budget.
const MaxThresholds = 8

const (
	maxMonthly = 1e12
	maxNameLen = 80
)

// DefaultThresholds are the alert levels, in percent of the monthly amount.
var DefaultThresholds = spendcap.DefaultThresholds

// ErrNotFound is returned when a budget id matches nothing.
var ErrNotFound = errors.New("costbudget: not found")

// Budget is one named monthly limit. Action is applied while spend is at or
// above 100 percent and reuses the spend cap actions, so nothing new can be
// destructive.
type Budget struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Scope       string  `json:"scope"`
	ScopeValue  string  `json:"scope_value"`
	Monthly     float64 `json:"monthly"`
	Thresholds  []int   `json:"thresholds"`
	Action      string  `json:"action"`
	ThrottleRPS int     `json:"throttle_rps"`
	GuardMode   string  `json:"guard_mode"`
}

// Normalize fills defaults and returns the budget ready to validate or store.
func (b Budget) Normalize() Budget {
	b.Name = strings.TrimSpace(b.Name)
	b.ScopeValue = strings.TrimSpace(b.ScopeValue)
	if b.Action == "" {
		b.Action = spendcap.ActionNone
	}
	if b.GuardMode == "" {
		b.GuardMode = spendcap.GuardAlert
	}
	if len(b.Thresholds) == 0 {
		b.Thresholds = append([]int(nil), DefaultThresholds...)
	}
	sorted := append([]int(nil), b.Thresholds...)
	sort.Ints(sorted)
	out := make([]int, 0, len(sorted))
	for i, t := range sorted {
		if i == 0 || t != sorted[i-1] {
			out = append(out, t)
		}
	}
	b.Thresholds = out
	if b.Action == spendcap.ActionThrottle && b.ThrottleRPS == 0 {
		b.ThrottleRPS = spendcap.DefaultThrottleRPS
	}
	if b.Action != spendcap.ActionThrottle {
		b.ThrottleRPS = 0
	}
	return b
}

// Validate rejects a budget the ticker could not act on sensibly.
func (b Budget) Validate() error {
	if b.Name == "" || len(b.Name) > maxNameLen {
		return fmt.Errorf("name must be 1 to %d characters", maxNameLen)
	}
	switch b.Scope {
	case ScopeProject, ScopeTag, ScopeApp:
	default:
		return errors.New("scope must be project, tag or app")
	}
	if b.ScopeValue == "" {
		return errors.New("scope_value is required")
	}
	if !(b.Monthly > 0) || b.Monthly > maxMonthly {
		return errors.New("monthly must be greater than zero")
	}
	if len(b.Thresholds) == 0 || len(b.Thresholds) > MaxThresholds {
		return fmt.Errorf("thresholds must list 1 to %d levels", MaxThresholds)
	}
	for _, t := range b.Thresholds {
		if t < 1 || t > 1000 {
			return errors.New("each threshold must be between 1 and 1000 percent")
		}
	}
	switch b.Action {
	case spendcap.ActionNone, spendcap.ActionThrottle, spendcap.ActionPause:
	default:
		return errors.New("action must be none, throttle or pause")
	}
	if err := spendcap.ValidateGuardMode(b.GuardMode); err != nil {
		return err
	}
	if b.GuardMode == spendcap.GuardBlockDeploys && b.Scope == ScopeTag {
		return errors.New("block_deploys supports app and project budgets, not tag budgets")
	}
	if b.Action == spendcap.ActionThrottle && (b.ThrottleRPS < 1 || b.ThrottleRPS > spendcap.MaxThrottleRPS) {
		return fmt.Errorf("throttle_rps must be 1 to %d", spendcap.MaxThrottleRPS)
	}
	return nil
}

// State is what the ticker last decided for a budget in a period.
type State struct {
	BudgetID     string    `json:"budget_id"`
	Period       string    `json:"period"`
	Highest      int       `json:"highest"`
	Pct          float64   `json:"pct"`
	Spent        float64   `json:"spent"`
	Enforced     string    `json:"enforced"`
	EnforcedApps []string  `json:"enforced_apps"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Period is the budget period key (UTC month) for t.
func Period(t time.Time) string { return spendcap.Period(t) }

// Result is one Step outcome.
type Result struct {
	State    State
	Fired    []int
	Rollover bool
	Changed  bool
}

// Pct returns spent as a percent of the monthly amount.
func Pct(spent, monthly float64) float64 {
	if monthly <= 0 {
		return 0
	}
	return spent / monthly * 100
}

// Step derives the next state from the previous one and current spend. A
// threshold fires once per period: only levels above the stored highest fire.
// Enforcement is level-triggered, so raising the budget lifts it again.
// Apps are the resolved members of the scope, recorded while enforced.
func Step(b Budget, prev State, hadPrev bool, now time.Time, spent float64, apps []string) Result {
	period := Period(now)
	st := prev
	res := Result{}
	if !hadPrev || prev.Period != period {
		res.Rollover = hadPrev
		st = State{BudgetID: b.ID, Period: period}
	}
	pct := Pct(spent, b.Monthly)
	for _, t := range b.Thresholds {
		if t > st.Highest && pct >= float64(t) {
			res.Fired = append(res.Fired, t)
			st.Highest = t
		}
	}
	enforced := ""
	var enforcedApps []string
	if pct >= 100 && b.Action != spendcap.ActionNone {
		enforced = b.Action
		enforcedApps = append([]string(nil), apps...)
		sort.Strings(enforcedApps)
	}
	st.Pct, st.Spent, st.Enforced, st.EnforcedApps, st.UpdatedAt = pct, spent, enforced, enforcedApps, now
	res.State = st
	res.Changed = !hadPrev || res.Rollover || len(res.Fired) > 0 || prev.Enforced != enforced ||
		strings.Join(prev.EnforcedApps, ",") != strings.Join(enforcedApps, ",")
	return res
}

// Breach is one fired threshold, keyed so a new level notifies again.
type Breach struct {
	Key  string
	Text string
}

// BreachFor describes the highest threshold a budget crossed this period.
func BreachFor(b Budget, st State, currency string) (Breach, bool) {
	if st.Highest == 0 {
		return Breach{}, false
	}
	text := fmt.Sprintf("budget %q (%s %s) has used %.0f%%: %.2f of %.2f %s", b.Name, b.Scope, b.ScopeValue, st.Pct, st.Spent, b.Monthly, currency)
	if st.Enforced == "" && st.Pct >= 100 && b.GuardMode == spendcap.GuardBlockDeploys {
		text += "; new deploys of the apps in scope are blocked until next month or until the budget is raised"
	}
	switch st.Enforced {
	case spendcap.ActionPause:
		text += "; the apps in scope are paused until next month or until the budget is raised"
	case spendcap.ActionThrottle:
		text += fmt.Sprintf("; each client is limited to %d requests per second", b.ThrottleRPS)
	}
	return Breach{Key: fmt.Sprintf("costbudget:%s:%s:%d", b.ID, st.Period, st.Highest), Text: text}, true
}
