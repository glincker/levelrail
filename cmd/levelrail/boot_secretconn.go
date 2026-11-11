package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/secretconn"
	"github.com/GLINCKER/levelrail/internal/secretprovider"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

// wireSecretConnections builds the named connection service, points the reference length
// limits at runtime settings and registers every connection as a source on boot.
func wireSecretConnections(rt *api.Router, db *store.DB, mgr *secrets.Manager, logger *slog.Logger) *secretconn.Service {
	svc := rt.RuntimeSettings()
	if mgr == nil {
		return nil
	}
	if svc != nil {
		secretsource.SetLimitsSource(func() secretsource.Limits {
			ctx := context.Background()
			return secretsource.Limits{
				MaxRef:      svc.Int(ctx, runtimesettings.KeySecretRefMaxRef),
				MaxConn:     svc.Int(ctx, runtimesettings.KeySecretRefMaxConn),
				MaxPath:     svc.Int(ctx, runtimesettings.KeySecretRefMaxPath),
				MaxSegments: svc.Int(ctx, runtimesettings.KeySecretRefMaxSegments),
				MaxKey:      svc.Int(ctx, runtimesettings.KeySecretRefMaxKey),
			}
		})
	}
	conns := &secretconn.Service{
		Rows: db, Secrets: mgr, Vault: db, Sources: secretsource.NewResolver(),
		VaultRes: vault.NewResolverWith(vault.TunablesFrom(svc)), ProvRes: secretprovider.Default(),
		ProvTun: secretprovider.TunablesFrom(svc),
	}
	rt.SetSecretConnections(conns)
	if err := conns.Sync(context.Background()); err != nil {
		logger.Warn("secret connections: initial registration failed", slog.String("error", err.Error()))
	}
	return conns
}
