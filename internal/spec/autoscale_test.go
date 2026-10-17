package spec

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/autoscale"
)

func TestAutoscalePolicy(t *testing.T) {
	tests := []struct {
		name    string
		in      Autoscale
		ceiling int
		wantErr string
		check   func(autoscale.Policy) bool
	}{
		{"defaults fill in", Autoscale{Max: 5, Metric: "cpu_percent", Target: 60}, 20, "", func(p autoscale.Policy) bool {
			return p.MinReplicas == 1 && p.ScaleUpCooldownSecs == 60 && p.ScaleDownCooldownSecs == 600 && p.StabilizationSecs == 300
		}},
		{"durations parse", Autoscale{Min: 2, Max: 5, Metric: "requests_per_replica", Target: 80, ScaleUpCooldown: "90s", ScaleDownCooldown: "15m", Stabilization: "1h"}, 20, "", func(p autoscale.Policy) bool {
			return p.ScaleUpCooldownSecs == 90 && p.ScaleDownCooldownSecs == 900 && p.StabilizationSecs == 3600
		}},
		{"bad duration", Autoscale{Max: 5, Metric: "cpu_percent", Target: 60, Stabilization: "soon"}, 20, "stabilization", nil},
		{"max above the ceiling", Autoscale{Max: 50, Metric: "cpu_percent", Target: 60}, 20, "must not exceed", nil},
		{"no max", Autoscale{Metric: "cpu_percent", Target: 60}, 20, "max_replicas", nil},
		{"bad metric", Autoscale{Max: 5, Metric: "disk", Target: 60}, 20, "metric", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.in.Policy("web", tc.ceiling)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.check != nil && !tc.check(p) {
				t.Errorf("policy = %+v", p)
			}
		})
	}
}

func TestAutoscaleValidate(t *testing.T) {
	var none *Autoscale
	if err := none.Validate("web", 3); err != nil {
		t.Errorf("a nil block is valid: %v", err)
	}
	block := &Autoscale{Max: 4, Metric: "memory_percent", Target: 70}
	if err := block.Validate("web", 0); err != nil {
		t.Errorf("valid block rejected: %v", err)
	}
	if err := block.Validate("web", 2); err == nil || !strings.Contains(err.Error(), "replicas and autoscale") {
		t.Errorf("replicas together with autoscale must be refused, got %v", err)
	}
}
