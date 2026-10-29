package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startBackupCopies runs the second-destination copy worker.
func startBackupCopies(ctx context.Context, logger *slog.Logger, db *store.DB, secrets backup.SecretsResolver) {
	runner := &backup.CopyRunner{
		Store:       db,
		Secrets:     secrets,
		Downloader:  backup.S3Downloader{},
		Uploader:    backup.S3Uploader{},
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
