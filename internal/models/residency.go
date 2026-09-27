package models

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envIdleTTL         = "APP_MODEL_IDLE_TTL"
	envMinIdleTTL      = "APP_MODEL_MIN_IDLE_TTL"
	defaultMinIdleTTL  = time.Minute
	defaultIdleTTL     = 15 * time.Minute
	maxIdleTTLSeconds  = 30 * 24 * 3600
	reasonIdle         = "Idle"
	reasonWakingUp     = "WakingUp"
	reasonWaitingOnGPU = "WaitingForGPU"
)

// DefaultIdleTTL reads APP_MODEL_IDLE_TTL.
func DefaultIdleTTL() time.Duration {
	if d := envDuration(envIdleTTL, defaultIdleTTL); d > 0 {
		return d
	}
	return defaultIdleTTL
}

// minIdleTTL reads APP_MODEL_MIN_IDLE_TTL. It keeps a TTL above the
// gateway's activity write interval so a busy model is never judged idle.
func minIdleTTL() time.Duration {
	if d := envDuration(envMinIdleTTL, defaultMinIdleTTL); d > 0 {
		return d
	}
	return defaultMinIdleTTL
}

// IdleTTL is how long an on-demand model may go unused before its engine
// stops: its own setting, else the platform default.
func IdleTTL(m store.Model) time.Duration {
	if m.IdleTTLSeconds > 0 {
		return time.Duration(m.IdleTTLSeconds) * time.Second
	}
	return DefaultIdleTTL()
}

// ResidencyActive reports whether an on-demand model should have its
// engine running at now: it was used or woken within its idle TTL.
func ResidencyActive(m store.Model, now time.Time) bool {
	return !m.LastActiveAt.IsZero() && now.Sub(m.LastActiveAt) < IdleTTL(m)
}

func validateResidency(residency string, idleTTL time.Duration) (string, int, error) {
	switch residency {
	case "", store.ResidencyAlways:
		residency = store.ResidencyAlways
	case store.ResidencyOnDemand:
	default:
		return "", 0, fmt.Errorf("%w: residency must be %q or %q", ErrInvalid, store.ResidencyAlways, store.ResidencyOnDemand)
	}
	if idleTTL < 0 || idleTTL > maxIdleTTLSeconds*time.Second {
		return "", 0, fmt.Errorf("%w: idle_ttl must be between 0 and 30 days", ErrInvalid)
	}
	if floor := minIdleTTL(); idleTTL > 0 && idleTTL < floor {
		return "", 0, fmt.Errorf("%w: idle_ttl must be at least %s", ErrInvalid, floor)
	}
	return residency, int(idleTTL / time.Second), nil
}

// SetResidency changes a model's residency mode and idle TTL (0 keeps the
// platform default). Switching to on-demand counts as activity.
func (s *Service) SetResidency(ctx context.Context, name, residency string, idleTTL time.Duration) error {
	mode, secs, err := validateResidency(residency, idleTTL)
	if err != nil {
		return err
	}
	return s.store.SetModelResidency(ctx, name, mode, secs, time.Now().UTC())
}

// Wake marks an on-demand model active so its engine starts now.
func (s *Service) Wake(ctx context.Context, name string) error {
	if err := s.requireOnDemand(ctx, name); err != nil {
		return err
	}
	return s.store.TouchModel(ctx, name, time.Now().UTC())
}

// Sleep marks an on-demand model idle so its engine stops on the next
// reconcile pass.
func (s *Service) Sleep(ctx context.Context, name string) error {
	if err := s.requireOnDemand(ctx, name); err != nil {
		return err
	}
	return s.store.SleepModel(ctx, name)
}

func (s *Service) requireOnDemand(ctx context.Context, name string) error {
	m, err := s.store.GetModel(ctx, name)
	if err != nil {
		return err
	}
	if m.Residency != store.ResidencyOnDemand {
		return fmt.Errorf("%w: model %q is always resident; set residency to on_demand first", ErrInvalid, name)
	}
	return nil
}
