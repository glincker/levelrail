package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// channelSecretSealer is the slice of *alerting.DB the startup seal needs.
type channelSecretSealer interface {
	SetChannelSecrets(cs alerting.ChannelSecrets)
	SealNotificationChannelSecrets(ctx context.Context) (int, error)
	CountPlaintextNotificationChannels(ctx context.Context) (int, error)
}

// setupNotificationChannelSecrets seals channel credentials and encrypts plaintext rows from older releases. Never fatal: without a master key, doctor warns.
func setupNotificationChannelSecrets(ctx context.Context, logger *slog.Logger, db channelSecretSealer, mgr *secrets.Manager) {
	if mgr == nil {
		if n, err := db.CountPlaintextNotificationChannels(ctx); err == nil && n > 0 {
			logger.Warn("notification channel credentials are stored in plaintext: no master key is configured", slog.Int("channels", n))
		}
		return
	}
	db.SetChannelSecrets(mgr)
	n, err := db.SealNotificationChannelSecrets(ctx)
	if err != nil {
		logger.Error("encrypting notification channel credentials failed, will retry on next start", slog.Int("sealed", n), slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		logger.Info("encrypted plaintext notification channel credentials", slog.Int("channels", n))
	}
}
