package main

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// authEngineOptions builds the library auth engine the control plane runs on.
// A failure is fatal to the caller: there is no other sign-in path.
func authEngineOptions(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *sql.DB, mgr *secrets.Manager) ([]api.Option, error) {
	cfg := authengine.Config{
		TokenPrefix:    b.ShortName,
		TOTPIssuer:     b.Name,
		Directory:      authengine.NewDirectory(db),
		DeviceTokenTTL: api.DeviceTokenTTL(),
		DeviceCodeTTL:  api.DeviceCodeTTL(),
		Sessions:       authSessionsHooks(db, logger),
		MFA:            authengine.MFAConfig{DashboardURL: authengine.LoadDashboardURL(ctx, db)},
	}
	if dial := dashboardDialAddr(httpAddr()); dial != "" {
		cfg.BaseURL = "http://" + dial
	}
	if mgr != nil {
		key, err := authengine.LoadOrCreateKey(ctx, mgr, true)
		if err != nil {
			logger.Error("auth engine: encryption key unavailable, TOTP stays off", slog.String("error", err.Error()))
		} else {
			cfg.EncryptionKey = key
		}
		sdb := &store.DB{DB: db}
		cfg.OAuth = &authengine.OAuthWiring{Settings: sdb, Secrets: mgr, Users: sdb, Logger: logger}
	}
	eng, err := authengine.New(db, authengine.ConfigFromEnv(cfg))
	if err != nil {
		return nil, err
	}
	logger.Info("auth engine ready", slog.String("prefix", eng.Prefix()))
	return []api.Option{api.WithAuthEngine(eng)}, nil
}
