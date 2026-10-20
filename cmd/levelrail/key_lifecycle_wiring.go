package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/keyreminder"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startKeyLifecycle enables the encryption key routes and the max-age
// reminder. Skipped without a master key: nothing to rotate then.
func startKeyLifecycle(ctx context.Context, logger *slog.Logger, db *store.DB, manager *secrets.Manager, channels *alerting.DB, notifier keyreminder.Notifier, router *api.Router) {
	if manager == nil {
		return
	}
	router.SetEncryptionKeys(secrets.NewDEKRotator(manager, db, logger), db)
	reminder := &keyreminder.Reminder{Source: keyReminderSource{db: db, manager: manager}, Channels: channels, Notifier: notifier, Logger: logger}
	go func() {
		if err := reminder.Run(ctx, keyreminder.DefaultInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("key reminder stopped", slog.String("error", err.Error()))
		}
	}()
}

type keyReminderSource struct {
	db      *store.DB
	manager *secrets.Manager
}

func (s keyReminderSource) GetSecretKeyPolicy(ctx context.Context) (store.SecretKeyPolicy, error) {
	return s.db.GetSecretKeyPolicy(ctx)
}

func (s keyReminderSource) MarkSecretKeyReminded(ctx context.Context, at time.Time) error {
	return s.db.MarkSecretKeyReminded(ctx, at)
}

func (s keyReminderSource) ListServiceDEKStatus(ctx context.Context) ([]store.ServiceDEKStatus, error) {
	return s.db.ListServiceDEKStatus(ctx)
}

func (s keyReminderSource) GetMasterKeyRotatedAt(ctx context.Context) (time.Time, bool, error) {
	return s.manager.GetMasterKeyRotatedAt(ctx)
}
