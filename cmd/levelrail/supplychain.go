package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
)

type supplyChainResolver struct {
	db  *store.DB
	sql *supplychain.SQLStore
}

func (r supplyChainResolver) AppExists(ctx context.Context, app string) (bool, error) {
	_, err := r.db.GetDesiredService(ctx, app)
	if errors.Is(err, store.ErrServiceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load app %q: %w", app, err)
	}
	return true, nil
}

func (r supplyChainResolver) AttemptExists(ctx context.Context, attemptID string) (bool, error) {
	return r.sql.AttemptExists(ctx, attemptID)
}

// newSupplyChainService builds the SBOM and scan service and starts its sweep.
func newSupplyChainService(ctx context.Context, logger *slog.Logger, db *store.DB, client *docker.Client, dataDir, namespace string) *supplychain.Service {
	cfg := supplychain.ConfigFromEnv(os.LookupEnv, logger)
	sqlStore := supplychain.NewSQLStore(db)
	svc := supplychain.New(cfg, supplychain.Deps{
		Store:     sqlStore,
		Resolver:  supplyChainResolver{db: db, sql: sqlStore},
		Runner:    client,
		DataDir:   dataDir,
		Namespace: namespace,
		Logger:    logger,
	})
	svc.Start(ctx)
	return svc
}

func supplyChainDeployOptions(svc *supplychain.Service) []deploy.Option {
	if svc == nil {
		return nil
	}
	return []deploy.Option{deploy.WithBuildAttest(svc.Config().BuildAttest), deploy.WithSupplyChain(svc)}
}
