package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseCredentialsResource is GET /api/v1/databases/{name}/credentials'
// wire shape: everything an operator needs to plug this database into an
// external client (TablePlus, DBeaver, redis-cli run from their own
// machine) without shelling into the container first.
// internal/reconcile/application's own resolveDatabaseField already
// resolves exactly these fields one at a time for
// DesiredService.DatabaseEnv/DatabaseAttachment, so this handler mirrors
// that logic directly (same internal/reconcile/database.SupportsField/
// ContainerName/ContainerPort/PasswordSecretKey helpers that package's
// own doc comment names as its cross-package "single source of truth"),
// just returning every field the engine supports in one response instead
// of resolving one at a time into a container's env.
type databaseCredentialsResource struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	URL      string `json:"url"`
}

// handleGetDatabaseCredentials handles GET
// /api/v1/databases/{name}/credentials. AbilityReadSensitive-gated
// (routes_platform.go's route registration): this discloses a real
// secret's plaintext, the same tier backup download already uses for
// the identical reason.
func (rt *Router) handleGetDatabaseCredentials(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get database credentials: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	port, ok := database.ContainerPort(desired.Engine)
	if !ok {
		writeError(w, http.StatusInternalServerError, "unrecognized database engine")
		return
	}
	host := database.ContainerName(name)

	creds := databaseCredentialsResource{Host: host, Port: port}

	if database.SupportsField(desired.Engine, "database") {
		creds.Database = name
	}
	if database.SupportsField(desired.Engine, "username") {
		creds.Username = name
	}
	if database.SupportsField(desired.Engine, "password") {
		password, err := rt.resolveDatabasePassword(r.Context(), name, desired.Engine)
		if err != nil {
			rt.logger.Error("api: get database credentials: resolve password failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		creds.Password = password
	}

	connectionURL, err := rt.resolveDatabaseURL(r.Context(), name, desired.Engine, host, port)
	if err != nil {
		rt.logger.Error("api: get database credentials: resolve url failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	creds.URL = connectionURL

	writeJSON(w, http.StatusOK, creds)
}

// resolveDatabasePassword mirrors internal/reconcile/application.
// Controller.resolveDatabasePassword exactly, using rt.composeSecrets
// (the same *internal/secrets.Manager instance, wired through
// WithComposeSecrets, ComposeSecretStore.Resolve's identical
// (ctx, name, key) shape) in place of that controller's own
// secretResolver field: both ultimately read the same stored secret.
func (rt *Router) resolveDatabasePassword(ctx context.Context, dbName, engine string) (string, error) {
	secretKey, ok := database.PasswordSecretKey(engine)
	if !ok {
		return "", fmt.Errorf("%s databases have no password", engine)
	}
	if rt.composeSecrets == nil {
		return "", fmt.Errorf("database %q needs a secret resolver to resolve its password but none is configured", dbName)
	}
	password, err := rt.composeSecrets.Resolve(ctx, dbName, secretKey)
	if err != nil {
		return "", fmt.Errorf("resolve password for database %q: %w", dbName, err)
	}
	return password, nil
}

// resolveDatabaseURL mirrors internal/reconcile/application.Controller.
// resolveDatabaseURL exactly, including its Redis/KeyDB/Dragonfly
// passwordless-scheme special case; see that method's own doc comment
// for why.
func (rt *Router) resolveDatabaseURL(ctx context.Context, dbName, engine, host string, port int) (string, error) {
	if engine == store.EngineRedis || engine == store.EngineKeyDB || engine == store.EngineDragonfly {
		return fmt.Sprintf("redis://%s:%d", host, port), nil
	}
	password, err := rt.resolveDatabasePassword(ctx, dbName, engine)
	if err != nil {
		return "", err
	}
	u := url.URL{
		Scheme: engine,
		User:   url.UserPassword(dbName, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + dbName,
	}
	return u.String(), nil
}
