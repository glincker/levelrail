package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	router.SetControlPlaneDR(svc, escrowMaterialReader(masterKeyFilePath, dataDir, os.Getenv))
	go svc.Run(ctx)
	if interval := cpbackup.ReplicaInterval(os.LookupEnv); interval > 0 {
		svc.Replica = &cpbackup.Replicator{Svc: svc, DBPath: filepath.Join(dataDir, storeFilename), Interval: interval}
		go svc.Replica.Run(ctx)
		logger.Info("control plane continuous replica enabled", slog.Duration("interval", interval))
	}
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

// After rotating an env-sourced key, update the env value before building a new escrow bundle.
func escrowMaterialReader(masterKeyFilePath, dataDir string, getenv func(string) string) api.EscrowMaterialReader {
	return func() (cpbackup.EscrowMaterial, error) {
		key, err := readMasterKey(masterKeyFilePath, getenv)
		if err != nil {
			return cpbackup.EscrowMaterial{}, err
		}
		files := map[string]string{}
		for _, name := range []string{agentCACertFilename, agentCAKeyFilename} {
			data, err := os.ReadFile(filepath.Join(dataDir, name)) //nolint:gosec // fixed file names in the operator data dir
			if err == nil {
				files[name] = string(data)
			}
		}
		return cpbackup.EscrowMaterial{MasterKey: key, Files: files}, nil
	}
}

func readMasterKey(masterKeyFilePath string, getenv func(string) string) (string, error) {
	if masterKeyFilePath != "" {
		data, err := os.ReadFile(masterKeyFilePath) //nolint:gosec // path chosen by the control plane at startup
		if err != nil {
			return "", fmt.Errorf("read master key file: %w", err)
		}
		return openMasterKeyMaterial(context.Background(), strings.TrimSpace(string(data)), getenv)
	}
	if v := strings.TrimSpace(getenv("APP_MASTER_KEY")); v != "" {
		return openMasterKeyMaterial(context.Background(), v, getenv)
	}
	return "", errors.New("no master key source is configured")
}
