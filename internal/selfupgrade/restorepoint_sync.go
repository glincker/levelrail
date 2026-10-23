package selfupgrade

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

// RestorePointStore is the persistence SyncRestorePoints needs; *store.DB satisfies it.
type RestorePointStore interface {
	ReplaceUpgradeRestorePoints(ctx context.Context, pts []store.UpgradeRestorePoint) error
}

// SyncRestorePoints makes the database table mirror the points kept on disk.
func SyncRestorePoints(ctx context.Context, rp *RestorePoints, db RestorePointStore) error {
	pts, err := rp.List()
	if err != nil {
		return err
	}
	rows := make([]store.UpgradeRestorePoint, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, store.UpgradeRestorePoint{
			ID: p.ID, Version: p.Version, SchemaVersion: p.Schema, AttemptID: p.AttemptID, SizeBytes: p.SizeBytes,
			HasBinary: p.HasBinary, HasUnit: p.HasUnit, CreatedAt: p.CreatedAt.UTC().Format(timeLayout),
		})
	}
	if err := db.ReplaceUpgradeRestorePoints(ctx, rows); err != nil {
		return fmt.Errorf("sync restore points: %w", err)
	}
	return nil
}
