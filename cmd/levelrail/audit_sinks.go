package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/auditstream"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// setupAuditStream starts the audit log streaming worker. Without a master
// key, webhook and S3 sinks cannot be created because credentials need it.
func setupAuditStream(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, secretsManager *secrets.Manager, router *api.Router) {
	deps := auditstream.Deps{
		Store:  db,
		Config: auditstream.ConfigFromEnv(os.LookupEnv),
		Brand:  b.ShortName,
		Logger: logger,
	}
	if secretsManager != nil {
		deps.Secrets = secretsManager
	}
	svc := auditstream.NewService(deps)
	router.SetAuditSinks(svc)
	go svc.Run(ctx)
}
