package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
)

const defaultGateSweepInterval = time.Hour

// setupBuiltinGate configures the built-in auth gate provider and returns
// the per-process secret the ingress presents to the check endpoint.
// Per process is enough: the ingress config is rebuilt on every start.
func setupBuiltinGate(ctx context.Context, logger *slog.Logger, rt *api.Router) string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		logger.Error("built-in auth gate disabled: generate secret failed", slog.String("error", err.Error()))
		return ""
	}
	secret := hex.EncodeToString(buf)
	settings, err := api.GateSettingsFromEnv(os.Getenv)
	if err != nil {
		logger.Warn("invalid built-in auth gate setting, using defaults", slog.String("error", err.Error()))
	}
	rt.SetGate(secret, settings)
	go func() {
		if err := rt.RunGateSweeper(ctx, envDurationOr(logger, "APP_GATE_SWEEP_INTERVAL", defaultGateSweepInterval)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("auth gate sweeper stopped", slog.String("error", err.Error()))
		}
	}()
	return secret
}
