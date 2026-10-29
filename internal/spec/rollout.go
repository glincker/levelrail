package spec

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/rolloutpolicy"
)

// Rollout is a service's rolling deploy and disruption budget settings.
// Each value is an int ("2") or a percent of replicas ("25%"); unset
// fields fall back to the APP_ROLLOUT_* env defaults.
type Rollout struct {
	MaxSurge       string `yaml:"max_surge,omitempty"`
	MaxUnavailable string `yaml:"max_unavailable,omitempty"`
	MinAvailable   string `yaml:"min_available,omitempty"`
}

// Policy converts the block to its rolloutpolicy form; nil is empty.
func (r *Rollout) Policy() rolloutpolicy.Policy {
	if r == nil {
		return rolloutpolicy.Policy{}
	}
	return rolloutpolicy.Policy{MaxSurge: r.MaxSurge, MaxUnavailable: r.MaxUnavailable, MinAvailable: r.MinAvailable}
}

// Validate checks the block's values; nil is valid.
func (r *Rollout) Validate(name string) error {
	if r == nil {
		return nil
	}
	if err := r.Policy().Validate(); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	return nil
}
