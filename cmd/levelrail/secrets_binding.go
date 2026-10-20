package main

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

// secretsManagerOptions configures slot binding enforcement. Legacy
// unbound values stay readable unless APP_SECRETS_REQUIRE_BOUND is true,
// which is only safe once `secrets rebind` reports nothing remaining.
func secretsManagerOptions(logger *slog.Logger) []secrets.ManagerOption {
	return []secrets.ManagerOption{
		secrets.WithLogger(logger),
		secrets.WithRequireBound(secretsRequireBound(logger)),
	}
}

func secretsRequireBound(logger *slog.Logger) bool {
	raw := os.Getenv("APP_SECRETS_REQUIRE_BOUND")
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warn("invalid APP_SECRETS_REQUIRE_BOUND, defaulting to false", slog.String("value", raw), slog.String("error", err.Error()))
		return false
	}
	return v
}
