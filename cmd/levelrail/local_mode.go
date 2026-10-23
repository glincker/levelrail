package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/localmode"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applyLocalMode enforces the loopback-only rules of a local instance and serves its apps over plain HTTP.
func applyLocalMode(ctx context.Context, logger *slog.Logger, db *store.DB) error {
	if !localmode.Enabled(os.Getenv) {
		return nil
	}
	for _, addr := range []string{httpAddr(), ingressHTTPAddr(), ingressHTTPSAddr(), agentAddr()} {
		if err := localmode.ValidateListenAddr(addr); err != nil {
			return fmt.Errorf("local mode: %w", err)
		}
	}
	if os.Getenv("APP_PUBLIC_HOST") == "" {
		if err := os.Setenv("APP_PUBLIC_HOST", ingress.LocalHost); err != nil {
			return fmt.Errorf("local mode: set public host: %w", err)
		}
	}
	settings, err := db.GetIngressSettings(ctx)
	if err != nil {
		return fmt.Errorf("local mode: read ingress settings: %w", err)
	}
	if !settings.TLSTerminatedUpstream {
		settings.TLSTerminatedUpstream = true
		if err := db.UpdateIngressSettings(ctx, settings); err != nil {
			return fmt.Errorf("local mode: serve apps over plain http: %w", err)
		}
	}
	logger.Info("local mode: loopback only, apps are served at <app>.localhost over plain http")
	return nil
}
