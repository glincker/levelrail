package rolloutpolicy

import (
	"errors"
	"testing"
)

func TestRolloutResolve(t *testing.T) {
	env := func(m map[string]string) Env { return func(k string) string { return m[k] } }
	tests := []struct {
		name     string
		p        Policy
		env      map[string]string
		replicas int
		want     Resolved
		waves    int
	}{
		{"fallback one at a time", Policy{}, nil, 3, Resolved{MaxSurge: 1}, 3},
		{"env defaults", Policy{}, map[string]string{EnvMaxSurge: "2", EnvMaxUnavailable: "1"}, 6, Resolved{MaxSurge: 2, MaxUnavailable: 1}, 2},
		{"percent rounding", Policy{MaxSurge: "25%", MaxUnavailable: "25%", MinAvailable: "50%"}, nil, 5, Resolved{MaxSurge: 2, MaxUnavailable: 1, MinAvailable: 3}, 2},
		{"bad env ignored", Policy{}, map[string]string{EnvMaxSurge: "lots"}, 2, Resolved{MaxSurge: 1}, 2},
		{"zero percents keep progress", Policy{MaxSurge: "10%", MaxUnavailable: "0"}, nil, 2, Resolved{MaxSurge: 1}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.p, env(tt.env), tt.replicas)
			if got != tt.want {
				t.Fatalf("Resolve = %+v, want %+v", got, tt.want)
			}
			if w := got.Waves(tt.replicas); w != tt.waves {
				t.Fatalf("Waves = %d, want %d", w, tt.waves)
			}
		})
	}
}

func TestRolloutValidateAndBudget(t *testing.T) {
	if err := (Policy{MaxSurge: "0", MaxUnavailable: "0%"}).Validate(); err == nil {
		t.Fatal("both zero should be rejected")
	}
	if err := (Policy{MaxSurge: "150%"}).Validate(); err == nil {
		t.Fatal("percent over 100 should be rejected")
	}
	if err := (Policy{MaxSurge: "2", MinAvailable: "50%"}).Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	var de *DisruptionError
	if err := CheckDisruption("web", "restart --all", 2, 3, 3); !errors.As(err, &de) || de.Remaining != 0 {
		t.Fatalf("want DisruptionError, got %v", err)
	}
	if err := CheckDisruption("web", "stop", 2, 3, 1); err != nil {
		t.Fatalf("within budget refused: %v", err)
	}
	if err := CheckDisruption("web", "drain", 0, 1, 1); err != nil {
		t.Fatalf("no budget refused: %v", err)
	}
}
