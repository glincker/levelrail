package main

import (
	"database/sql"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// authEngineOAuthWiring returns the OAuth wiring when the oauth area is served by the
// library and the secret store is available; nil keeps the in-house OAuth sign-in.
func authEngineOAuthWiring(logger *slog.Logger, db *sql.DB, mgr *secrets.Manager) *authengine.OAuthWiring {
	if !authengine.AreaActive(authengine.AreaOAuth) || mgr == nil || db == nil {
		return nil
	}
	sdb := &store.DB{DB: db}
	return &authengine.OAuthWiring{Settings: sdb, Secrets: mgr, Users: sdb, Logger: logger}
}

func authEngineOAuthOptions(eng *authengine.Engine) []api.Option {
	if !eng.OAuthEnabled() {
		return nil
	}
	return []api.Option{api.WithAuthLibOAuth(eng)}
}
