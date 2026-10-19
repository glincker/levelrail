package costbudget

import (
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestStepThresholds(t *testing.T) {
	b := Budget{ID: "b1", Name: "web", Scope: ScopeApp, ScopeValue: "web", Monthly: 100, Action: spendcap.ActionPause}.Normalize()
	apps := []string{"web"}
	steps := []struct {
		name      string
		now       string
		spent     float64
		wantFired []int
		wantEnf   string
		wantRoll  bool
	}{
		{"under all levels", "2026-10-02 10:00", 20, nil, "", false},
		{"crosses 50", "2026-10-05 10:00", 55, []int{50}, "", false},
		{"same level does not refire", "2026-10-06 10:00", 60, nil, "", false},
		{"jump crosses 80 and 100", "2026-10-09 10:00", 130, []int{80, 100}, spendcap.ActionPause, false},
		{"still over, no refire", "2026-10-10 10:00", 130, nil, spendcap.ActionPause, false},
		{"rollover resets and fires 50 again", "2026-11-03 10:00", 60, []int{50}, "", true},
	}
	var st State
	had := false
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			res := Step(b, st, had, at(s.now), s.spent, apps)
			if !reflect.DeepEqual(res.Fired, s.wantFired) {
				t.Fatalf("fired = %v, want %v", res.Fired, s.wantFired)
			}
			if res.State.Enforced != s.wantEnf || res.Rollover != s.wantRoll {
				t.Fatalf("enforced = %q rollover = %v", res.State.Enforced, res.Rollover)
			}
			st, had = res.State, true
		})
	}
}

func TestStepRestartDoesNotRefire(t *testing.T) {
	b := Budget{ID: "b1", Name: "n", Scope: ScopeTag, ScopeValue: "prod", Monthly: 100}.Normalize()
	first := Step(b, State{}, false, at("2026-10-05 10:00"), 85, nil)
	if !reflect.DeepEqual(first.Fired, []int{50, 80}) {
		t.Fatalf("first fired = %v", first.Fired)
	}
	again := Step(b, first.State, true, at("2026-10-05 10:02"), 86, nil)
	if len(again.Fired) != 0 || again.State.Highest != 80 {
		t.Fatalf("restart refired: %+v", again)
	}
}

func TestStepEnforcementIsLevelTriggered(t *testing.T) {
	b := Budget{ID: "b1", Name: "n", Scope: ScopeProject, ScopeValue: "p", Monthly: 100, Action: spendcap.ActionThrottle}.Normalize()
	over := Step(b, State{}, false, at("2026-10-05 10:00"), 120, []string{"b", "a"})
	if over.State.Enforced != spendcap.ActionThrottle || !reflect.DeepEqual(over.State.EnforcedApps, []string{"a", "b"}) {
		t.Fatalf("over = %+v", over.State)
	}
	b.Monthly = 200
	lifted := Step(b, over.State, true, at("2026-10-05 11:00"), 120, []string{"a", "b"})
	if lifted.State.Enforced != "" || len(lifted.State.EnforcedApps) != 0 || !lifted.Changed {
		t.Fatalf("raising the budget should lift: %+v", lifted)
	}
}

func TestValidate(t *testing.T) {
	ok := Budget{Name: "n", Scope: ScopeApp, ScopeValue: "web", Monthly: 10}.Normalize()
	cases := []struct {
		name string
		mut  func(*Budget)
		bad  bool
	}{
		{"valid", func(*Budget) {}, false},
		{"no name", func(b *Budget) { b.Name = "" }, true},
		{"bad scope", func(b *Budget) { b.Scope = "team" }, true},
		{"zero monthly", func(b *Budget) { b.Monthly = 0 }, true},
		{"bad action", func(b *Budget) { b.Action = "delete" }, true},
		{"threshold zero", func(b *Budget) { b.Thresholds = []int{0} }, true},
		{"throttle default rps", func(b *Budget) {
			*b = Budget{Name: "n", Scope: ScopeTag, ScopeValue: "t", Monthly: 1, Action: spendcap.ActionThrottle}.Normalize()
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := ok
			c.mut(&b)
			if err := b.Validate(); (err != nil) != c.bad {
				t.Fatalf("err = %v, bad = %v", err, c.bad)
			}
		})
	}
}
