package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spendguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

func spendGuardDeployOptions(db *store.DB, logger *slog.Logger) []deploy.Option {
	g := &spendguard.Guard{
		Store:  db,
		Logger: logger,
		ProjectOf: func(ctx context.Context, app string) (string, error) {
			svc, err := db.GetDesiredService(ctx, app)
			if err != nil {
				return "", fmt.Errorf("load app: %w", err)
			}
			return svc.ProjectID, nil
		},
	}
	return []deploy.Option{deploy.WithSpendGuard(g)}
}
