package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// GFSStore is the store surface grandfather-father-son pruning needs.
type GFSStore interface {
	ListSucceededVolumeBackups(ctx context.Context, serviceName, volumeName string) ([]store.BackupHistory, error)
	DeleteBackupHistory(ctx context.Context, id string) error
}

// pruneGFS removes the succeeded backups of one volume that its daily,
// weekly and monthly policy no longer wants, objects first and rows after, so
// a failed object delete leaves a row the next run retries.
func (s *Scheduler) pruneGFS(ctx context.Context, v store.ServiceVolumeBackupConfig) {
	if s.GFS == nil || s.Policies == nil {
		return
	}
	label := volumeScheduleKey(v.ServiceName, v.VolumeName)
	policy, err := s.Policies.GetVolumeBackupPolicy(ctx, v.ServiceName, v.VolumeName)
	if errors.Is(err, store.ErrVolumeBackupPolicyNotFound) || (err == nil && !policy.HasRetention()) {
		return
	}
	if err != nil {
		s.log().Error("backup: scheduled volume retention: load policy failed", slog.String("service_volume", label), slog.String("error", err.Error()))
		return
	}
	rows, err := s.GFS.ListSucceededVolumeBackups(ctx, v.ServiceName, v.VolumeName)
	if err != nil {
		s.log().Error("backup: scheduled volume retention: list backups failed", slog.String("service_volume", label), slog.String("error", err.Error()))
		return
	}
	items := make([]RetentionItem, 0, len(rows))
	byID := make(map[string]store.BackupHistory, len(rows))
	for _, h := range rows {
		at, perr := time.Parse(time.RFC3339, h.StartedAt)
		if perr != nil {
			continue
		}
		items = append(items, RetentionItem{ID: h.ID, At: at})
		byID[h.ID] = h
	}
	expired := ExpiredByPolicy(items, RetentionPolicy{Daily: policy.RetainDaily, Weekly: policy.RetainWeekly, Monthly: policy.RetainMonthly})
	for _, id := range expired {
		if err := s.deleteBackup(ctx, byID[id]); err != nil {
			s.log().Error("backup: scheduled volume retention: delete failed",
				slog.String("service_volume", label), slog.String("id", id), slog.String("error", err.Error()))
		}
	}
	if len(expired) > 0 {
		s.log().Info("backup: scheduled volume retention pruned backups", slog.String("service_volume", label), slog.Int("deleted", len(expired)))
	}
}

func (s *Scheduler) deleteBackup(ctx context.Context, h store.BackupHistory) error {
	if s.Deleter != nil {
		dest, err := s.Runner.ResolveDestination(ctx, h.TargetID)
		if err != nil {
			return fmt.Errorf("resolve destination: %w", err)
		}
		if err := s.Deleter.Delete(ctx, dest, h.ObjectKey); err != nil {
			return fmt.Errorf("delete object %q: %w", h.ObjectKey, err)
		}
	}
	return s.GFS.DeleteBackupHistory(ctx, h.ID)
}
