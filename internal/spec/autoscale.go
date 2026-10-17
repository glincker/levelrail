package spec

import (
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
)

// Autoscale is a service's replica autoscaling policy (internal/autoscale).
// It owns the replica count, so it cannot be combined with replicas:.
type Autoscale struct {
	Min    int     `yaml:"min"`
	Max    int     `yaml:"max"`
	Metric string  `yaml:"metric"`
	Target float64 `yaml:"target"`
	// Tolerances are percents of Target: scale up above it, scale down below it.
	ScaleUpTolerance   int `yaml:"scale_up_tolerance,omitempty"`
	ScaleDownTolerance int `yaml:"scale_down_tolerance,omitempty"`
	// Durations use Go syntax such as 90s or 10m.
	ScaleUpCooldown   string `yaml:"scale_up_cooldown,omitempty"`
	ScaleDownCooldown string `yaml:"scale_down_cooldown,omitempty"`
	Stabilization     string `yaml:"stabilization,omitempty"`
	MaxStepUp         int    `yaml:"max_step_up,omitempty"`
	MaxStepDown       int    `yaml:"max_step_down,omitempty"`
}

func seconds(field, v string) (int, error) {
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < time.Second {
		return 0, fmt.Errorf("autoscale.%s: %q is not a duration of at least 1s", field, v)
	}
	return int(d / time.Second), nil
}

// Policy converts the block into a normalized, validated policy for app.
// ceiling is the global maximum replicas, 0 to skip that check.
func (a *Autoscale) Policy(app string, ceiling int) (autoscale.Policy, error) {
	p := autoscale.Policy{
		App: app, MinReplicas: a.Min, MaxReplicas: a.Max, Metric: a.Metric, Target: a.Target,
		ScaleUpTolerancePct: a.ScaleUpTolerance, ScaleDownTolerancePct: a.ScaleDownTolerance,
		MaxStepUp: a.MaxStepUp, MaxStepDown: a.MaxStepDown,
	}
	var err error
	if p.ScaleUpCooldownSecs, err = seconds("scale_up_cooldown", a.ScaleUpCooldown); err != nil {
		return p, err
	}
	if p.ScaleDownCooldownSecs, err = seconds("scale_down_cooldown", a.ScaleDownCooldown); err != nil {
		return p, err
	}
	if p.StabilizationSecs, err = seconds("stabilization", a.Stabilization); err != nil {
		return p, err
	}
	if p.MinReplicas == 0 {
		p.MinReplicas = 1
	}
	p = p.Normalize()
	if err := p.Validate(ceiling); err != nil {
		return p, fmt.Errorf("autoscale: %w", err)
	}
	return p, nil
}

// Validate checks the block; a nil block is valid.
func (a *Autoscale) Validate(service string, replicas int) error {
	if a == nil {
		return nil
	}
	if replicas != 0 {
		return fmt.Errorf("spec: service %q: %w", service, errors.New("replicas and autoscale cannot both be set: autoscale owns the replica count, use autoscale.min for the floor"))
	}
	if _, err := a.Policy(service, 0); err != nil {
		return fmt.Errorf("spec: service %q: %w", service, err)
	}
	return nil
}
