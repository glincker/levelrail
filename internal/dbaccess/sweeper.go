package dbaccess

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// TempCredential is one tracked short-lived login. Its secret is never held.
type TempCredential struct {
	ID        string
	Database  string
	Role      string
	Preset    Preset
	CreatedBy string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SweepStore is the persistence the sweeper needs. Claim must be an atomic
// compare-and-set so overlapping passes revoke each credential once.
type SweepStore interface {
	// DueTempCredentials lists credentials past ExpiresAt that are not yet revoked.
	DueTempCredentials(ctx context.Context, now time.Time) ([]TempCredential, error)
	// ClaimTempCredential moves one to revoking. A stale claim from a
	// crashed pass is reclaimable after lease. False means someone else has it.
	ClaimTempCredential(ctx context.Context, id string, now time.Time, lease time.Duration) (bool, error)
	CompleteTempCredential(ctx context.Context, id string, now time.Time) error
	// ReleaseTempCredential returns a claimed credential for the next pass.
	ReleaseTempCredential(ctx context.Context, id string) error
}

// Revoker drops one temporary role. It must be idempotent.
type Revoker interface {
	RevokeTemp(ctx context.Context, database, role string) error
}

// SweepAuditor records that the system revoked a credential.
type SweepAuditor interface {
	RecordRevoked(ctx context.Context, c TempCredential)
}

// Sweeper revokes temporary credentials at expiry, on a timer and once at
// boot, so a control plane restart never extends a credential's life.
type Sweeper struct {
	Store   SweepStore
	Revoker Revoker
	Auditor SweepAuditor
	Logger  *slog.Logger
	Now     func() time.Time
	// Lease is how long a revoking claim is honoured before another pass retakes it.
	Lease time.Duration
}

const defaultLease = 2 * time.Minute

func (s *Sweeper) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Run sweeps immediately, then every interval until ctx ends.
func (s *Sweeper) Run(ctx context.Context, interval time.Duration) error {
	s.Sweep(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep runs one pass and returns how many credentials it revoked.
func (s *Sweeper) Sweep(ctx context.Context) int {
	now := s.now()
	due, err := s.Store.DueTempCredentials(ctx, now)
	if err != nil {
		s.Logger.Error("dbaccess: list due temporary credentials failed", slog.String("error", err.Error()))
		return 0
	}
	lease := s.Lease
	if lease <= 0 {
		lease = defaultLease
	}
	done := 0
	for _, c := range due {
		claimed, err := s.Store.ClaimTempCredential(ctx, c.ID, now, lease)
		if err != nil || !claimed {
			if err != nil {
				s.Logger.Error("dbaccess: claim temporary credential failed", slog.String("credential_id", c.ID), slog.String("error", err.Error()))
			}
			continue
		}
		if err := s.Revoker.RevokeTemp(ctx, c.Database, c.Role); err != nil {
			s.Logger.Warn("dbaccess: revoke temporary credential failed, will retry",
				slog.String("credential_id", c.ID), slog.String("database", c.Database), slog.String("error", err.Error()))
			if rerr := s.Store.ReleaseTempCredential(ctx, c.ID); rerr != nil && !errors.Is(rerr, context.Canceled) {
				s.Logger.Error("dbaccess: release temporary credential failed", slog.String("credential_id", c.ID), slog.String("error", rerr.Error()))
			}
			continue
		}
		if err := s.Store.CompleteTempCredential(ctx, c.ID, s.now()); err != nil {
			s.Logger.Error("dbaccess: mark temporary credential revoked failed", slog.String("credential_id", c.ID), slog.String("error", err.Error()))
			continue
		}
		if s.Auditor != nil {
			s.Auditor.RecordRevoked(ctx, c)
		}
		s.Logger.Info("dbaccess: temporary credential revoked", slog.String("credential_id", c.ID), slog.String("database", c.Database), slog.String("role", c.Role))
		done++
	}
	return done
}
