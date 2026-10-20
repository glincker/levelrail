package backup

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

const tombstoneSweepBatch = 50

// TombstoneStore remembers objects whose delete failed after their row was removed.
type TombstoneStore interface {
	AddBackupTombstone(ctx context.Context, targetID, objectKey, reason string) error
	ListBackupTombstones(ctx context.Context, limit int) ([]store.BackupTombstone, error)
	ResolveBackupTombstone(ctx context.Context, id int64) error
	BumpBackupTombstone(ctx context.Context, id int64, reason string) error
}

func recordTombstone(ctx context.Context, ts TombstoneStore, log *slog.Logger, targetID, objectKey string, cause error) {
	if ts == nil || objectKey == "" {
		return
	}
	if err := ts.AddBackupTombstone(ctx, targetID, objectKey, cause.Error()); err != nil {
		log.Error("backup: could not record object tombstone, object may be orphaned",
			slog.String("target_id", targetID), slog.String("object_key", objectKey), slog.String("error", err.Error()))
	}
}

// sweepTombstones retries pending object deletes and clears the ones that succeed.
func (s *Scheduler) sweepTombstones(ctx context.Context) {
	if s.Tombstones == nil || s.Deleter == nil {
		return
	}
	pending, err := s.Tombstones.ListBackupTombstones(ctx, tombstoneSweepBatch)
	if err != nil {
		s.log().Error("backup: list object tombstones failed", slog.String("error", err.Error()))
		return
	}
	for _, t := range pending {
		dest, derr := s.Runner.ResolveDestination(ctx, t.TargetID)
		if derr == nil {
			derr = s.Deleter.Delete(ctx, dest, t.ObjectKey)
		}
		if derr != nil {
			_ = s.Tombstones.BumpBackupTombstone(ctx, t.ID, derr.Error())
			continue
		}
		if err := s.Tombstones.ResolveBackupTombstone(ctx, t.ID); err != nil {
			s.log().Error("backup: clear object tombstone failed", slog.Int64("id", t.ID), slog.String("error", err.Error()))
		}
	}
}
