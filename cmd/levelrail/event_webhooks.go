package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/brand"
	"github.com/glincker/levelrail/internal/eventhook"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/secrets"
	"github.com/glincker/levelrail/internal/store"
)

// setupEventWebhooks starts the outbound event webhook workers. Without a
// master key, webhooks cannot be created because signing secrets need it.
func setupEventWebhooks(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, secretsManager *secrets.Manager, router *api.Router) {
	deps := eventhook.Deps{
		Store:  db,
		Config: eventhook.ConfigFromEnv(os.LookupEnv),
		RetentionDays: func(c context.Context) int {
			return router.RuntimeSettings().Int(c, runtimesettings.KeyEventWebhookDays)
		},
		Brand:  b.ShortName,
		Logger: logger,
	}
	if secretsManager != nil {
		deps.Secrets = secretsManager
	}
	svc := eventhook.NewService(deps)
	router.SetEventWebhooks(svc)
	go svc.Run(ctx)
}
