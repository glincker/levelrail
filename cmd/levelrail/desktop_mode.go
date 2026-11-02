package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/desktopmode"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// desktopModeEnabled reads APP_DESKTOP_MODE. It is a bootstrap env, not a runtime
// setting: the Host and Origin checks must wrap the server before settings load.
func desktopModeEnabled() bool {
	return desktopmode.Enabled(os.Getenv)
}

// wrapDesktopMode returns next unchanged unless desktop mode is on. When it is on
// and cannot start, it fails closed with 503 rather than serving without the checks.
func wrapDesktopMode(logger *slog.Logger, db *store.DB, rt *api.Router, next http.Handler) http.Handler {
	if !desktopModeEnabled() {
		return next
	}
	dir := dataDirFromEnv()
	tokens, err := desktopmode.NewTokens(dir)
	if err != nil {
		logger.Error("desktop mode: token unavailable, serving nothing", slog.String("error", err.Error()))
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "desktop mode unavailable", http.StatusServiceUnavailable)
		})
	}
	settings := rt.RuntimeSettings()
	if settings == nil {
		settings = runtimesettings.New(db, nil)
	}
	ports := append([]string{dashboardPort()}, strings.Split(os.Getenv("APP_DESKTOP_ALLOWED_PORTS"), ",")...)
	svc := desktopmode.New(desktopmode.Config{
		Tokens:   tokens,
		Sessions: rt.LibSessions(),
		Admin:    func(ctx context.Context) (authengine.LegacyUser, error) { return desktopAdmin(ctx, db) },
		TTL: func(ctx context.Context) time.Duration {
			return settings.Duration(ctx, runtimesettings.KeyDesktopSessionTTL)
		},
		Audit: func(ctx context.Context, e store.AuditEntry) {
			if err := db.SaveAuditEntry(ctx, e); err != nil {
				logger.Warn("desktop mode: save audit entry failed", slog.String("error", err.Error()))
			}
		},
		Logger:      logger,
		Ports:       ports,
		RatePerMin:  apiRateLimitRPM(logger, "APP_DESKTOP_EXCHANGE_RPM", 30),
		MaxFailures: apiRateLimitRPM(logger, "APP_DESKTOP_MAX_FAILURES", 5),
		Lockout:     envDurationOr(logger, "APP_DESKTOP_LOCKOUT", 5*time.Minute),
	})
	logger.Info("desktop mode: on", slog.String("token_file", desktopmode.TokenPath(dir)))
	return svc.Wrap(next)
}

// desktopAdmin is the install's root admin: the first user, falling back to the oldest.
func desktopAdmin(ctx context.Context, db *store.DB) (authengine.LegacyUser, error) {
	users, err := db.ListUsers(ctx)
	if err != nil {
		return authengine.LegacyUser{}, err
	}
	if len(users) == 0 {
		return authengine.LegacyUser{}, errors.New("no users")
	}
	pick := users[0]
	for _, u := range users {
		if u.IsFirstUser {
			pick = u
			break
		}
	}
	return toLibraryUser(pick), nil
}
