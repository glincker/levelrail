package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

var errAdminExists = errors.New("an admin account already exists, so there is no setup token; use recover-admin to regain access")

func dataDirFromEnv() string {
	if d := os.Getenv("APP_DATA_DIR"); d != "" {
		return d
	}
	return defaultDataDir
}

// setupURLPath is the login path that pre-fills the setup form with token.
func setupURLPath(token string) string {
	return "/login?setup=" + token
}

func dashboardPort() string {
	_, port, err := net.SplitHostPort(httpAddr())
	if err != nil || port == "" {
		return "8080"
	}
	return port
}

// ensureSetupToken creates the first-admin setup token when no user exists
// yet, logging it only when newly generated.
func ensureSetupToken(ctx context.Context, logger *slog.Logger, db *store.DB) {
	token, created, err := api.EnsureSetupToken(ctx, db, dataDirFromEnv())
	if err != nil {
		logger.Error("setup token not available: first-admin registration is closed", slog.String("error", err.Error()))
		return
	}
	switch {
	case token == "":
		return
	case created:
		logger.Warn("no admin account exists yet: open the dashboard and create one with this setup token",
			slog.String("setup_token", token),
			slog.String("path", setupURLPath(token)),
		)
	default:
		logger.Warn("no admin account exists yet: print the setup token with the setup-token subcommand",
			slog.String("file", api.SetupTokenPath(dataDirFromEnv())),
		)
	}
}

// runSetupToken is the setup-token subcommand: it prints the current
// first-admin setup token, creating one if the file is missing.
func runSetupToken(ctx context.Context, stdout io.Writer, openStore func(context.Context) (*store.DB, error)) error {
	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	token, _, err := api.EnsureSetupToken(ctx, db, dataDirFromEnv())
	if err != nil {
		return err
	}
	if token == "" {
		return errAdminExists
	}
	host := publicHost()
	if host == "" {
		host = "<server-ip>"
	}
	_, _ = fmt.Fprintf(stdout, "setup token: %s\nopen: http://%s%s\n", token, net.JoinHostPort(host, dashboardPort()), setupURLPath(token))
	return nil
}

// allowInsecureLogin reads APP_ALLOW_INSECURE_LOGIN, the recovery escape
// hatch for the plain-HTTP login refusal.
func allowInsecureLogin(logger *slog.Logger) bool {
	raw := os.Getenv("APP_ALLOW_INSECURE_LOGIN")
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warn("invalid APP_ALLOW_INSECURE_LOGIN, defaulting to disabled", slog.String("value", raw), slog.String("error", err.Error()))
		return false
	}
	return v
}
