package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/backup"
	"github.com/glincker/levelrail/internal/store"
)

// startBackupCopies runs the second-destination copy worker. storage routes local and node
// destinations, which have no S3 endpoint.
func startBackupCopies(ctx context.Context, logger *slog.Logger, db *store.DB, secrets backup.SecretsResolver, storage backup.Routed) {
	runner := &backup.CopyRunner{
		Store:       db,
		Secrets:     secrets,
		Downloader:  storage,
		Uploader:    storage,
		Concurrency: backup.CopyConcurrencyFromEnv(os.LookupEnv),
		Logger:      logger,
	}
	go func() {
		if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("backup copy worker stopped", slog.String("error", err.Error()))
		}
	}()
}

// backupCopyAlertSource feeds kind=backup_copy_behind rules from the copy table.
type backupCopyAlertSource struct{ db *store.DB }

// PendingBackupCopies implements alerting.BackupCopySource.
func (s backupCopyAlertSource) PendingBackupCopies(ctx context.Context) ([]alerting.BackupCopyPending, error) {
	rows, err := s.db.ListUncopiedBackupCopies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]alerting.BackupCopyPending, 0, len(rows))
	for _, c := range rows {
		at, err := time.Parse(time.RFC3339, c.BackupFinished)
		if err != nil {
			continue
		}
		out = append(out, alerting.BackupCopyPending{Resource: c.ResourceName, BackupID: c.BackupHistoryID, FinishedAt: at, LastError: c.Error})
	}
	return out, nil
}

// backupSizeAlertSource feeds kind=backup_size_anomaly rules from recent backup history.
type backupSizeAlertSource struct{ db *store.DB }

// backupSizeHistoryRows bounds the history read; older rows cannot matter to a small baseline.
const backupSizeHistoryRows = 2000

// RecentBackupSizes implements alerting.BackupSizeSource.
func (s backupSizeAlertSource) RecentBackupSizes(ctx context.Context) ([]alerting.BackupSizeSeries, error) {
	rows, err := s.db.ListAllBackupHistory(ctx, backupSizeHistoryRows, nil)
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	var out []alerting.BackupSizeSeries
	for _, h := range rows {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		name := h.DatabaseName
		if h.ResourceKind == store.BackupResourceKindVolume {
			name = h.ServiceName + "/" + h.VolumeName
		}
		i, ok := idx[name]
		if !ok {
			i = len(out)
			idx[name] = i
			out = append(out, alerting.BackupSizeSeries{Resource: name})
		}
		out[i].Sizes = append(out[i].Sizes, h.SizeBytes)
	}
	return out, nil
}
