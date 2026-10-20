package spendcap

import (
	"reflect"
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	const gb = int64(1) << 30
	base := Cap{App: "web", MonthlyEgressBytes: 100 * gb, MonthlyCostCeiling: 50, Thresholds: []int{50, 80, 100}, Action: ActionPause}
	tests := []struct {
		name      string
		cap       Cap
		use       Usage
		pct       float64
		crossed   []int
		exhausted bool
		desired   string
	}{
		{"idle", base, Usage{}, 0, nil, false, ActionNone},
		{"below first", base, Usage{EgressBytes: 49 * gb}, 49, nil, false, ActionNone},
		{"exactly 50", base, Usage{EgressBytes: 50 * gb}, 50, []int{50}, false, ActionNone},
		{"cost binds", base, Usage{EgressBytes: 10 * gb, Cost: 42}, 84, []int{50, 80}, false, ActionNone},
		{"egress exhausted", base, Usage{EgressBytes: 100 * gb}, 100, []int{50, 80, 100}, true, ActionPause},
		{"over", base, Usage{EgressBytes: 250 * gb}, 250, []int{50, 80, 100}, true, ActionPause},
		{"alert only", Cap{App: "web", MonthlyEgressBytes: gb, Thresholds: []int{100}, Action: ActionNone}, Usage{EgressBytes: 2 * gb}, 200, []int{100}, true, ActionNone},
		{"egress only ignores cost", Cap{App: "web", MonthlyEgressBytes: gb, Thresholds: []int{50}, Action: ActionThrottle}, Usage{Cost: 9999}, 0, nil, false, ActionNone},
		{"cost only", Cap{App: "web", MonthlyCostCeiling: 10, Thresholds: []int{100}, Action: ActionThrottle}, Usage{Cost: 10}, 100, []int{100}, true, ActionThrottle},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Evaluate(tc.cap, tc.use)
			if e.Pct != tc.pct || e.Exhausted != tc.exhausted || e.Desired != tc.desired || !reflect.DeepEqual(e.Crossed, tc.crossed) {
				t.Errorf("got pct=%v crossed=%v exhausted=%v desired=%s", e.Pct, e.Crossed, e.Exhausted, e.Desired)
			}
		})
	}
}

func TestNormalizeAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      Cap
		wantErr bool
		check   func(Cap) bool
	}{
		{"defaults", Cap{App: "a", MonthlyEgressBytes: 1}, false, func(c Cap) bool {
			return c.Action == ActionNone && reflect.DeepEqual(c.Thresholds, DefaultThresholds)
		}},
		{"sorts and dedupes", Cap{App: "a", MonthlyEgressBytes: 1, Thresholds: []int{90, 10, 90}}, false, func(c Cap) bool {
			return reflect.DeepEqual(c.Thresholds, []int{10, 90})
		}},
		{"throttle default rps", Cap{App: "a", MonthlyEgressBytes: 1, Action: ActionThrottle}, false, func(c Cap) bool { return c.ThrottleRPS == DefaultThrottleRPS }},
		{"rps dropped for pause", Cap{App: "a", MonthlyEgressBytes: 1, Action: ActionPause, ThrottleRPS: 9}, false, func(c Cap) bool { return c.ThrottleRPS == 0 }},
		{"no limit", Cap{App: "a"}, true, nil},
		{"negative egress", Cap{App: "a", MonthlyEgressBytes: -1}, true, nil},
		{"negative cost", Cap{App: "a", MonthlyCostCeiling: -1}, true, nil},
		{"no app", Cap{MonthlyEgressBytes: 1}, true, nil},
		{"bad action", Cap{App: "a", MonthlyEgressBytes: 1, Action: "explode"}, true, nil},
		{"threshold zero", Cap{App: "a", MonthlyEgressBytes: 1, Thresholds: []int{0}}, true, nil},
		{"threshold over 100", Cap{App: "a", MonthlyEgressBytes: 1, Thresholds: []int{101}}, true, nil},
		{"too many thresholds", Cap{App: "a", MonthlyEgressBytes: 1, Thresholds: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}}, true, nil},
		{"rps too high", Cap{App: "a", MonthlyEgressBytes: 1, Action: ActionThrottle, ThrottleRPS: MaxThrottleRPS + 1}, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.in.Normalize()
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.check != nil && !tc.check(c) {
				t.Errorf("normalized cap wrong: %+v", c)
			}
		})
	}
}

func TestPeriodRollover(t *testing.T) {
	tests := []struct {
		at     time.Time
		period string
		next   time.Time
	}{
		{time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC), "2026-10", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), "2026-11", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), "2026-12", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 10, 31, 23, 0, 0, 0, time.FixedZone("x", -5*3600)), "2026-11", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		if got := Period(tc.at); got != tc.period {
			t.Errorf("Period(%v) = %s, want %s", tc.at, got, tc.period)
		}
		if got := NextPeriodStart(tc.at); !got.Equal(tc.next) {
			t.Errorf("NextPeriodStart(%v) = %v, want %v", tc.at, got, tc.next)
		}
	}
}

func TestConditions(t *testing.T) {
	cp := Cap{App: "a", ThrottleRPS: 5}
	tests := []struct {
		name             string
		eval             Eval
		enforced, nudged string
		reason           string
		healthy          bool
	}{
		{"within", Eval{}, "", "", ReasonWithinBudget, true},
		{"threshold", Eval{Pct: 60, Crossed: []int{50}}, "", "", ReasonThresholdReached, true},
		{"alert only", Eval{Pct: 120, Exhausted: true, Crossed: []int{100}}, "", "", ReasonLimitAlertOnly, false},
		{"throttled", Eval{Pct: 120, Exhausted: true}, ActionThrottle, ActionThrottle, ReasonThrottled, false},
		{"paused", Eval{Pct: 120, Exhausted: true}, ActionPause, ActionPause, ReasonPaused, false},
		{"pending", Eval{Pct: 120, Exhausted: true}, ActionPause, "", ReasonEnforcementPending, false},
		{"release pending", Eval{}, "", ActionPause, ReasonEnforcementPending, false},
	}
	for _, tc := range tests {
		got := Conditions(cp, tc.eval, tc.enforced, tc.nudged)
		if got.Reason != tc.reason || got.Healthy != tc.healthy || got.Message == "" {
			t.Errorf("%s: %+v", tc.name, got)
		}
	}
}
