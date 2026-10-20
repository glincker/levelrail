package splittest

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

func variants(wa, wb int) []Variant {
	return []Variant{
		{ID: "a1", Label: "Control", Kind: KindProduction, Weight: wa},
		{ID: "b2", Label: "Redesign", Kind: KindBranch, Ref: "redesign", Weight: wb},
	}
}

func TestValidate(t *testing.T) {
	three := append(variants(50, 30), Variant{ID: "c3", Label: "Other", Kind: KindRelease, Ref: "web-r1", Weight: 20})
	tests := []struct {
		name    string
		vs      []Variant
		wantErr bool
	}{
		{"ok 50/50", variants(50, 50), false},
		{"ok three", three, false},
		{"sum under", variants(40, 50), true},
		{"sum over", variants(60, 50), true},
		{"negative", variants(110, -10), true},
		{"single", variants(100, 0)[:1], true},
		{"no production", []Variant{{ID: "x", Label: "x", Kind: KindBranch, Ref: "a", Weight: 50}, {ID: "y", Label: "y", Kind: KindBranch, Ref: "b", Weight: 50}}, true},
		{"missing ref", []Variant{{ID: "x", Label: "x", Kind: KindProduction, Weight: 50}, {ID: "y", Label: "y", Kind: KindBranch, Weight: 50}}, true},
		{"dup label", []Variant{{ID: "x", Label: "A", Kind: KindProduction, Weight: 50}, {ID: "y", Label: "a", Kind: KindBranch, Ref: "b", Weight: 50}}, true},
		{"bad kind", []Variant{{ID: "x", Label: "A", Kind: KindProduction, Weight: 50}, {ID: "y", Label: "B", Kind: "nope", Ref: "b", Weight: 50}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.vs); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDecide(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // seeded for a deterministic test
	vs := variants(50, 50)
	tests := []struct {
		name       string
		vs         []Variant
		forced     string
		cookie     string
		wantID     string
		wantCookie bool
		anyID      bool
	}{
		{name: "cookie sticks", vs: vs, cookie: "b2", wantID: "b2"},
		{name: "cookie sticks other", vs: vs, cookie: "a1", wantID: "a1"},
		{name: "forced by label sets cookie", vs: vs, forced: "redesign", cookie: "a1", wantID: "b2", wantCookie: true},
		{name: "forced matching cookie keeps it", vs: vs, forced: "b2", cookie: "b2", wantID: "b2"},
		{name: "unknown forced falls to cookie", vs: vs, forced: "zzz", cookie: "b2", wantID: "b2"},
		{name: "stale cookie redraws", vs: vs, cookie: "gone", wantCookie: true, anyID: true},
		{name: "zero weight cookie redraws", vs: variants(100, 0), cookie: "b2", wantID: "a1", wantCookie: true},
		{name: "no cookie draws", vs: vs, wantCookie: true, anyID: true},
		{name: "small weight change keeps visitor", vs: variants(90, 10), cookie: "b2", wantID: "b2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.vs, tc.forced, tc.cookie, rng)
			if !tc.anyID && got.Variant.ID != tc.wantID {
				t.Fatalf("variant = %q, want %q", got.Variant.ID, tc.wantID)
			}
			if got.SetCookie != tc.wantCookie {
				t.Fatalf("SetCookie = %v, want %v", got.SetCookie, tc.wantCookie)
			}
		})
	}
}

func TestDrawDistribution(t *testing.T) {
	const draws = 100000
	tests := []struct{ wa, wb int }{{50, 50}, {90, 10}, {1, 99}, {100, 0}}
	for _, tc := range tests {
		rng := rand.New(rand.NewPCG(42, 7)) //nolint:gosec // seeded for a deterministic test
		vs := variants(tc.wa, tc.wb)
		counts := map[string]int{}
		for i := 0; i < draws; i++ {
			counts[Draw(vs, rng).ID]++
		}
		for _, v := range vs {
			want := float64(v.Weight) / 100
			got := float64(counts[v.ID]) / draws
			if math.Abs(got-want) > 0.01 {
				t.Errorf("weights %d/%d: variant %s share %.3f, want %.3f", tc.wa, tc.wb, v.ID, got, want)
			}
		}
	}
}

func TestDefaultsFromEnv(t *testing.T) {
	env := map[string]string{EnvCookieName: "ab_test", EnvCookieTTL: "48h", EnvOverrideParam: "bad name!", EnvFlushInterval: "5s"}
	d := DefaultsFromEnv(func(k string) string { return env[k] })
	if d.CookieName != "ab_test" || d.CookieTTL != 48*time.Hour || d.OverrideParam != "lr_variant" || d.FlushInterval != 5*time.Second {
		t.Fatalf("unexpected defaults: %+v", d)
	}
	empty := DefaultsFromEnv(func(string) string { return "" })
	if empty.CookieName != "lr_split" || empty.CookieTTL != 30*24*time.Hour {
		t.Fatalf("unexpected built-in defaults: %+v", empty)
	}
}
