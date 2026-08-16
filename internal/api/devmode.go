package api

import (
	"context"
	"log/slog"
	"net/http"
)

// devModeUsername and devModePassword are fixed and deliberately
// well-known: the build-tag gate in devmode_debug.go/devmode_release.go
// is what makes this safe, not credential secrecy, so there is no value
// in randomizing them. A founder iterating on the dashboard locally
// types "dev"/"dev" once per data-dir reset instead of remembering
// APP_ADMIN_USERNAME/APP_ADMIN_PASSWORD.
const (
	devModeUsername = "dev"
	devModePassword = "dev"
)

// maybeBootstrapDevAdmin is MaybeBootstrapDevAdmin's testable core:
// MaybeBootstrapDevAdmin below supplies the real devModeEnabled() gate,
// tests supply enabled directly. When enabled is false this is a pure
// no-op; requireAuth and the real login handler are never touched, this
// only ever affects whether a fixed account exists to log in as.
func maybeBootstrapDevAdmin(ctx context.Context, s AuthStore, logger *slog.Logger, enabled bool) error {
	if !enabled {
		return nil
	}
	logger.Warn("APP_DEV_MODE is enabled: bootstrapping fixed dev admin account, do not use this build in production",
		slog.String("username", devModeUsername),
		slog.String("password", devModePassword),
	)
	return BootstrapAdmin(ctx, s, devModeUsername, devModePassword)
}

// MaybeBootstrapDevAdmin is cmd/levelrail/main.go's entry point, called
// unconditionally on every startup the same way BootstrapAdmin already
// is: a no-op unless APP_DEV_MODE=1 in a non -tags embedweb build (see
// devmode_debug.go/devmode_release.go). BootstrapAdmin itself is a
// no-op once any admin account exists, so this only ever creates the
// fixed dev/dev account on a genuinely empty data directory; an operator
// who already set APP_ADMIN_USERNAME/APP_ADMIN_PASSWORD keeps that
// account regardless of call order between the two.
func MaybeBootstrapDevAdmin(ctx context.Context, s AuthStore, logger *slog.Logger) error {
	return maybeBootstrapDevAdmin(ctx, s, logger, devModeEnabled())
}

type devModeResponse struct {
	Enabled bool `json:"enabled"`
}

// handleDevMode serves GET /api/v1/dev-mode: public, no auth required,
// same "public, safe to expose" reasoning as handleBrand. Lets the login
// screen offer a one-click dev login (the fixed dev/dev credentials
// MaybeBootstrapDevAdmin already seeds) instead of the operator typing
// them by hand, without the frontend needing to guess whether dev mode
// is active. Reveals only a boolean, never the credentials themselves,
// those are already public knowledge documented in dev-fixtures.yml and
// this package's own devModeUsername/devModePassword constants, not a
// secret this endpoint could leak.
func (rt *Router) handleDevMode(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, devModeResponse{Enabled: devModeEnabled()})
}
