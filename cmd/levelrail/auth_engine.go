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
		BaseURL:        "http://localhost",
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
		if kp, err := authengine.LoadOrCreateSAMLKeyPair(ctx, mgr); err != nil {
			logger.Error("auth engine: SAML keypair unavailable, SAML sign-in stays off", slog.String("error", err.Error()))
		} else {
			cfg.SAML = kp
		}
		sdb := &store.DB{DB: db}
		cfg.OAuth = &authengine.OAuthWiring{Settings: sdb, Secrets: mgr, Users: sdb, Logger: logger, TrustedPeer: api.PeerIsTrustedProxy}
	}
	eng, err := authengine.New(db, authengine.ConfigFromEnv(cfg))
	if err != nil {
		return nil, err
	}
	logger.Info("auth engine ready", slog.String("prefix", eng.Prefix()))
	// Pay the one-time argon2id cost at boot so the first unknown-user sign-in is not slower.
	eng.Auth().WarmDummyHash()
	importLegacyTokens(ctx, logger, eng)
	opts := []api.Option{api.WithAuthEngine(eng)}
	if len(cfg.EncryptionKey) > 0 {
		opts = append(opts, api.WithLoginCodeKey(cfg.EncryptionKey))
	}
	return opts, nil
}

// importLegacyTokens carries owned tokens minted before the auth library into
// it. A failure is not fatal: those tokens still import on first use.
func importLegacyTokens(ctx context.Context, logger *slog.Logger, eng *authengine.Engine) {
	sum, err := eng.ImportLegacyTokens(ctx)
	if err != nil {
		logger.Error("legacy token import incomplete", slog.Int("pending", sum.Pending), slog.String("error", err.Error()))
	}
	if sum.Migrated > 0 || sum.Skipped > 0 {
		logger.Info("legacy tokens migrated", slog.Int("migrated", sum.Migrated), slog.Int("skipped", sum.Skipped))
	}
}
