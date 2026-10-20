package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/secretrotation"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startSecretRotation enables rotation policies and runs their sweep. It is a
// no-op without a master key: no secret exists to rotate.
func startSecretRotation(ctx context.Context, logger *slog.Logger, db *store.DB, router *api.Router, haveSecrets bool) {
	if !haveSecrets {
		return
	}
	maxPerPass := secretrotation.IntFromEnv(os.Getenv, secretrotation.EnvMaxPerPass, secretrotation.DefaultMaxPerRun)
	soonDays := secretrotation.IntFromEnv(os.Getenv, secretrotation.EnvDueSoonDays, int(secretrotation.DefaultDueSoon/(24*time.Hour)))
	interval := time.Duration(secretrotation.IntFromEnv(os.Getenv, secretrotation.EnvIntervalSecs, int(secretrotation.DefaultInterval/time.Second))) * time.Second
	engine := router.SetSecretRotation(db, maxPerPass, time.Duration(soonDays)*24*time.Hour)
	go func() {
		if err := engine.Run(ctx, interval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("secret rotation sweeper stopped", slog.String("error", err.Error()))
		}
	}()
}
