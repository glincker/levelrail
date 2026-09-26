package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

// setupControlPlaneDR starts scheduled off-box encrypted backups and restore
// drills and enables their API routes. It returns nil without a master key,
// because bucket credentials have nowhere safe to live.
func setupControlPlaneDR(ctx context.Context, logger *slog.Logger, db *store.DB, secretsManager *secrets.Manager, masterKeyFilePath, dataDir string, router *api.Router) *cpbackup.Service {
	if secretsManager == nil {
		return nil
	}
	resolver := &objectstore.Resolver{Store: db, Secrets: secretsManager}
	svc := cpbackup.NewService(db, db, cpbackup.ResolverDestinations{Resolver: resolver}, dataDir, version.Version, cpbackup.OptionsFromEnv(os.LookupEnv), logger)
	svc.DrillIdentities = drillIdentities(logger)
	router.SetControlPlaneDR(svc, masterKeyReader(masterKeyFilePath, os.Getenv))
	go svc.Run(ctx)
	return svc
}

func drillIdentities(logger *slog.Logger) []age.Identity {
	path := os.Getenv(cpbackup.EnvDrillIdentityFile)
	if path == "" {
		return nil
	}
	ids, err := cpbackup.ParseIdentityFile(path)
	if err != nil {
		logger.Warn("drill identity unusable, restore drills will only verify checksums", slog.String("error", err.Error()))
		return nil
	}
	return ids
}

// masterKeyReader returns the serialized master key from its file, or from
// APP_MASTER_KEY when the key is env-sourced. After rotating an env-sourced
// key, update the env value before building a new escrow bundle.
func masterKeyReader(masterKeyFilePath string, getenv func(string) string) api.MasterKeyReader {
	return func() (string, error) {
		if masterKeyFilePath != "" {
			data, err := os.ReadFile(masterKeyFilePath) //nolint:gosec // path chosen by the control plane at startup
			if err != nil {
				return "", fmt.Errorf("read master key file: %w", err)
			}
			return strings.TrimSpace(string(data)), nil
		}
		if v := strings.TrimSpace(getenv("APP_MASTER_KEY")); v != "" {
			return v, nil
		}
		return "", errors.New("no master key source is configured")
	}
}
