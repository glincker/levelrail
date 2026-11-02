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

// wrapDesktopMode returns next unchanged unless APP_DESKTOP_MODE is on. It is a bootstrap
// env, not a runtime setting: the Host and Origin checks must wrap the server before settings
// load. When it is on but unusable, or set to something unparsable, it fails closed with 503.
func wrapDesktopMode(logger *slog.Logger, db *store.DB, rt *api.Router, next http.Handler) http.Handler {
	on, err := desktopmode.Mode(os.Getenv)
	if err != nil {
		return desktopRefuse(logger, err)
	}
	if !on {
		return next
	}
	if _, err := desktopAdmin(context.Background(), db); errors.Is(err, desktopmode.ErrNotSingleUser) {
		return desktopRefuse(logger, err)
	}
	dir := dataDirFromEnv()
	tokens, err := desktopmode.NewTokens(dir)
	if err != nil {
		return desktopRefuse(logger, err)
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
		Logger:         logger,
		Ports:          ports,
		FailuresPerMin: apiRateLimitRPM(logger, "APP_DESKTOP_EXCHANGE_RPM", 10),
		Block:          envDurationOr(logger, "APP_DESKTOP_LOCKOUT", 30*time.Second),
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
	// Desktop mode hands the root admin session to the app, so any second user would be reachable from it.
	if len(users) > 1 || !users[0].IsFirstUser {
		return authengine.LegacyUser{}, desktopmode.ErrNotSingleUser
	}
	return toLibraryUser(users[0]), nil
}

// desktopRefuse serves 503 for everything, so a broken desktop setup never runs unguarded.
func desktopRefuse(logger *slog.Logger, err error) http.Handler {
	logger.Error("desktop mode: refusing to serve", slog.String("error", err.Error()))
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "desktop mode unavailable", http.StatusServiceUnavailable)
	})
}
