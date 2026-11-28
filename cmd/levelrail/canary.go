package main

import (
	"context"
	"log/slog"

	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/canary"
	"github.com/glincker/levelrail/internal/ingress"
	"github.com/glincker/levelrail/internal/store"
)

// startCanaryReconciler runs the staged-canary loop and gives the API its live
// stage comparison.
func startCanaryReconciler(ctx context.Context, logger *slog.Logger, db *store.DB, rt *api.Router) {
	rec := canary.NewReconciler(db, ingress.DefaultRequestStats(), canary.DefaultDials(), rt.CanaryActions(), canary.DefaultLimits(canary.OSEnv), nil, logger)
	rt.SetCanaryProgress(rec)
	if svc := rt.RuntimeSettings(); svc != nil {
		rec.SetLimitsSource(func() canary.Limits { return canary.DefaultLimits(canary.OSEnv) })
	}
	go rec.Run(ctx)
}
