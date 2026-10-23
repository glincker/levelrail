// Package spendcap holds per-app monthly spend and egress guardrails: the
// pure threshold math and the level-triggered controller step that decides
// whether an app is throttled or paused. Persistence and the ingress effect
// live with the callers.
package spendcap

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Hard actions taken when a limit is reached.
const (
	ActionNone     = "none"
	ActionThrottle = "throttle"
	ActionPause    = "pause"
)

// Limits on what an operator can configure.
const (
	DefaultThrottleRPS = 5
	MaxThrottleRPS     = 100000
	MaxThresholds      = 8
	maxEgressBytes     = int64(1) << 60
	maxCostCeiling     = 1e12
)

// Guard modes decide whether reaching the limit also refuses new deploys.
const (
	GuardAlert        = "alert"
	GuardBlockDeploys = "block_deploys"
)

// EnvWarnPercents overrides the default alert levels, for example "80,100".
const EnvWarnPercents = "APP_SPEND_WARN_PERCENTS"

// DefaultThresholds are the alert levels, in percent of the limit.
var DefaultThresholds = ParseWarnPercents(os.Getenv(EnvWarnPercents), []int{50, 80, 100})

// ParseWarnPercents reads comma separated whole percents. Anything invalid
// falls back, so a typo in the environment can never disable alerting.
func ParseWarnPercents(raw string, fallback []int) []int {
	if strings.TrimSpace(raw) == "" {
		return append([]int(nil), fallback...)
	}
	var out []int
	for _, p := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 || n > 100 || len(out) >= MaxThresholds {
			return append([]int(nil), fallback...)
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// ErrNotFound is returned when an app has no cap.
var ErrNotFound = errors.New("spendcap: not found")

// Cap is one app's monthly guardrail. A zero limit is unset.
type Cap struct {
	App                string  `json:"app"`
	MonthlyEgressBytes int64   `json:"monthly_egress_bytes"`
	MonthlyCostCeiling float64 `json:"monthly_cost_ceiling"`
	Thresholds         []int   `json:"thresholds"`
	Action             string  `json:"action"`
	ThrottleRPS        int     `json:"throttle_rps"`
	GuardMode          string  `json:"guard_mode"`
}

// Usage is an app's month to date consumption.
type Usage struct {
	EgressBytes int64
	Cost        float64
}

// Normalize fills defaults and returns the cap ready to validate or store.
func (c Cap) Normalize() Cap {
	if c.Action == "" {
		c.Action = ActionNone
	}
	if c.GuardMode == "" {
		c.GuardMode = GuardAlert
	}
	if len(c.Thresholds) == 0 {
		c.Thresholds = append([]int(nil), DefaultThresholds...)
	}
	sorted := append([]int(nil), c.Thresholds...)
	sort.Ints(sorted)
	out := make([]int, 0, len(sorted))
	for i, t := range sorted {
		if i == 0 || t != sorted[i-1] {
			out = append(out, t)
		}
	}
	c.Thresholds = out
	if c.Action == ActionThrottle && c.ThrottleRPS == 0 {
		c.ThrottleRPS = DefaultThrottleRPS
	}
	if c.Action != ActionThrottle {
		c.ThrottleRPS = 0
	}
	return c
}

// Validate rejects a cap the controller could not act on sensibly.
func (c Cap) Validate() error {
	switch {
	case c.App == "":
		return errors.New("app is required")
	case c.MonthlyEgressBytes < 0 || c.MonthlyEgressBytes > maxEgressBytes:
		return errors.New("monthly_egress_bytes is out of range")
	case math.IsNaN(c.MonthlyCostCeiling) || c.MonthlyCostCeiling < 0 || c.MonthlyCostCeiling > maxCostCeiling:
		return errors.New("monthly_cost_ceiling is out of range")
	case c.MonthlyEgressBytes == 0 && c.MonthlyCostCeiling == 0:
		return errors.New("set monthly_egress_bytes, monthly_cost_ceiling or both")
	}
	switch c.Action {
	case ActionNone, ActionThrottle, ActionPause:
	default:
		return fmt.Errorf("action must be %s, %s or %s", ActionNone, ActionThrottle, ActionPause)
	}
	if err := ValidateGuardMode(c.GuardMode); err != nil {
		return err
	}
	if c.Action == ActionThrottle && (c.ThrottleRPS < 1 || c.ThrottleRPS > MaxThrottleRPS) {
		return fmt.Errorf("throttle_rps must be between 1 and %d", MaxThrottleRPS)
	}
	if len(c.Thresholds) > MaxThresholds {
		return fmt.Errorf("at most %d thresholds", MaxThresholds)
	}
	for _, t := range c.Thresholds {
		if t < 1 || t > 100 {
			return errors.New("thresholds must be whole percents between 1 and 100")
		}
	}
	return nil
}

// ValidateGuardMode rejects an unknown guard mode.
func ValidateGuardMode(m string) error {
	if m != GuardAlert && m != GuardBlockDeploys {
		return fmt.Errorf("guard_mode must be %s or %s", GuardAlert, GuardBlockDeploys)
	}
	return nil
}

// Period is the UTC month key a usage window belongs to.
func Period(t time.Time) string { return t.UTC().Format("2006-01") }

// NextPeriodStart is the first instant of the month after t.
func NextPeriodStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

// Eval is the position of usage against a cap.
type Eval struct {
	EgressPct float64
	CostPct   float64
	Pct       float64
	Crossed   []int
	Exhausted bool
	// Desired is the action that should be in force right now.
	Desired string
}

// Evaluate positions usage against the cap. The binding limit is whichever
// is closest to exhaustion, and the hard action applies once it reaches 100%.
func Evaluate(c Cap, u Usage) Eval {
	var e Eval
	if c.MonthlyEgressBytes > 0 {
		e.EgressPct = float64(u.EgressBytes) / float64(c.MonthlyEgressBytes) * 100
	}
	if c.MonthlyCostCeiling > 0 {
		e.CostPct = u.Cost / c.MonthlyCostCeiling * 100
	}
	e.Pct = math.Max(e.EgressPct, e.CostPct)
	for _, t := range c.Thresholds {
		if e.Pct >= float64(t) {
			e.Crossed = append(e.Crossed, t)
		}
	}
	e.Exhausted = e.Pct >= 100
	e.Desired = ActionNone
	if e.Exhausted {
		e.Desired = c.Action
	}
	return e
}

// Highest is the largest crossed threshold, or 0.
func (e Eval) Highest() int {
	if len(e.Crossed) == 0 {
		return 0
	}
	return e.Crossed[len(e.Crossed)-1]
}

// Condition reasons.
const (
	ReasonWithinBudget       = "WithinBudget"
	ReasonThresholdReached   = "ThresholdReached"
	ReasonLimitAlertOnly     = "LimitReachedAlertOnly"
	ReasonThrottled          = "Throttled"
	ReasonPaused             = "Paused"
	ReasonDeploysBlocked     = "DeploysBlocked"
	ReasonEnforcementPending = "EnforcementPending"
)

// Condition is the controller's status for one app.
type Condition struct {
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Conditions derives the status. enforced is the stored action and nudged
// the one ingress was last told about; they differ while a change is still
// reaching ingress.
func Conditions(c Cap, e Eval, enforced, nudged string) Condition {
	switch {
	case enforced != nudged:
		return Condition{Reason: ReasonEnforcementPending, Message: fmt.Sprintf("Waiting for ingress to apply %q; retrying.", orNone(enforced))}
	case enforced == ActionPause:
		return Condition{Reason: ReasonPaused, Message: fmt.Sprintf("Limit reached at %.0f%%. The app serves a maintenance response until next month or until the limit is raised.", e.Pct)}
	case enforced == ActionThrottle:
		return Condition{Reason: ReasonThrottled, Message: fmt.Sprintf("Limit reached at %.0f%%. Each client is limited to %d requests per second until next month or until the limit is raised.", e.Pct, c.ThrottleRPS)}
	case e.Exhausted && c.GuardMode == GuardBlockDeploys:
		return Condition{Reason: ReasonDeploysBlocked, Message: fmt.Sprintf("Limit reached at %.0f%%. The app keeps serving, but new deploys are refused until next month, until the limit is raised or until a root user overrides.", e.Pct)}
	case e.Exhausted:
		return Condition{Reason: ReasonLimitAlertOnly, Message: fmt.Sprintf("Limit reached at %.0f%%. The action is alert only, so the app keeps serving.", e.Pct)}
	case len(e.Crossed) > 0:
		return Condition{Healthy: true, Reason: ReasonThresholdReached, Message: fmt.Sprintf("Usage is at %.0f%% of the limit.", e.Pct)}
	}
	return Condition{Healthy: true, Reason: ReasonWithinBudget, Message: fmt.Sprintf("Usage is at %.0f%% of the limit.", e.Pct)}
}

func orNone(s string) string {
	if s == "" {
		return ActionNone
	}
	return s
}
