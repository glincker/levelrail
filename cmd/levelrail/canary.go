package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/canary"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startCanaryReconciler runs the staged-canary loop and gives the API its live
// stage comparison.
func startCanaryReconciler(ctx context.Context, logger *slog.Logger, db *store.DB, rt *api.Router) {
	rec := canary.NewReconciler(db, ingress.DefaultRequestStats(), canary.DefaultDials(), rt.CanaryActions(), canary.DefaultLimits(canary.OSEnv), nil, logger)
	rt.SetCanaryProgress(rec)
	go rec.Run(ctx)
}
