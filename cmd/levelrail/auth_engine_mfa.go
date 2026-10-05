package main

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
)

// authEngineMFAOptions routes TOTP and passkeys through the library when AreaMFA is active.
func authEngineMFAOptions(logger *slog.Logger, eng *authengine.Engine, db *sql.DB) []api.Option {
	if !authengine.AreaActive(authengine.AreaMFA) {
		return nil
	}
	m, err := authengine.NewMFA(eng, db)
	if err != nil {
		logger.Error("auth engine: mfa setup failed, built-in TOTP and passkeys stay active", slog.String("error", err.Error()))
		return nil
	}
	logger.Info("auth engine: TOTP and passkeys served by the library",
		slog.Bool("totp", m.TOTPAvailable()), slog.Bool("passkeys", m.PasskeysAvailable()))
	return []api.Option{api.WithAuthEngineMFA(m)}
}

func authEngineMFADashboardURL(ctx context.Context, db *sql.DB) string {
	if !authengine.AreaActive(authengine.AreaMFA) {
		return ""
	}
	return authengine.LoadDashboardURL(ctx, db)
}
