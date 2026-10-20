package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretrotation"
)

const (
	envDatabasePasswordRotationIntervalSecs = "APP_DATABASE_PASSWORD_ROTATION_INTERVAL_SECONDS" //nolint:gosec // env var name, not a credential
	defaultDatabasePasswordRotationSecs     = 3600
)

func databasePasswordRotationInterval(_ *slog.Logger) time.Duration {
	secs := secretrotation.IntFromEnv(os.Getenv, envDatabasePasswordRotationIntervalSecs, defaultDatabasePasswordRotationSecs)
	return time.Duration(secs) * time.Second
}
