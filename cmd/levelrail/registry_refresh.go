package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/registrytoken"
	"github.com/glincker/levelrail/internal/secrets"
	"github.com/glincker/levelrail/internal/store"
)

// newRegistryRefresher builds the preset registry credential refresher.
func newRegistryRefresher(db *store.DB, mgr *secrets.Manager, logger *slog.Logger) *registrytoken.Refresher {
	settings := registrytoken.SettingsFromEnv(os.Getenv)
	return &registrytoken.Refresher{
		Store:    db,
		Secrets:  mgr,
		Minters:  registrytoken.DefaultMinters(settings.FallbackTTL),
		Settings: settings,
		Logger:   logger,
	}
}

// registryRefreshSource feeds kind=registry_refresh_failing alert rules.
type registryRefreshSource struct{ db *store.DB }

func (s registryRefreshSource) RegistryRefreshHealth(ctx context.Context) ([]alerting.RegistryRefreshHealth, error) {
	creds, err := s.db.ListRegistryCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("list registry credentials: %w", err)
	}
	var out []alerting.RegistryRefreshHealth
	for _, c := range creds {
		if c.Preset == "" {
			continue
		}
		out = append(out, alerting.RegistryRefreshHealth{Name: c.Name, Preset: c.Preset, LastError: c.LastRefreshError, ExpiresAt: c.ExpiresAt})
	}
	return out, nil
}
