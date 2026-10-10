package dbaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// StoreAdapter exposes the store's access-user table as a SweepStore.
type StoreAdapter struct{ DB *store.DB }

// DueTempCredentials implements SweepStore.
func (a StoreAdapter) DueTempCredentials(ctx context.Context, now time.Time) ([]TempCredential, error) {
	rows, err := a.DB.DueDatabaseAccessUsers(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("due temporary credentials: %w", err)
	}
	out := make([]TempCredential, 0, len(rows))
	for _, r := range rows {
		out = append(out, FromStore(r))
	}
	return out, nil
}

// FromStore converts a store record.
func FromStore(r store.DatabaseAccessUser) TempCredential {
	c := TempCredential{ID: r.ID, Database: r.Database, Role: r.Role, Preset: Preset(r.Preset), CreatedBy: r.CreatedBy}
	c.CreatedAt, _ = time.Parse(store.DatabaseAccessTimeLayout, r.CreatedAt)
	c.ExpiresAt, _ = time.Parse(store.DatabaseAccessTimeLayout, r.ExpiresAt)
	return c
}

// ClaimTempCredential implements SweepStore.
func (a StoreAdapter) ClaimTempCredential(ctx context.Context, id string, now time.Time, lease time.Duration) (bool, error) {
	return a.DB.ClaimDatabaseAccessUser(ctx, id, now, lease)
}

// CompleteTempCredential implements SweepStore.
func (a StoreAdapter) CompleteTempCredential(ctx context.Context, id string, now time.Time) error {
	return a.DB.CompleteDatabaseAccessUser(ctx, id, now)
}

// ReleaseTempCredential implements SweepStore.
func (a StoreAdapter) ReleaseTempCredential(ctx context.Context, id string) error {
	return a.DB.ReleaseDatabaseAccessUser(ctx, id)
}
