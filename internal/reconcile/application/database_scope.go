package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrDatabaseOutOfScope is wrapped when a service references a database
// whose network scope excludes it.
var ErrDatabaseOutOfScope = errors.New("database is out of network scope for this service")

// DatabaseScopeStore is the surface the controller needs to honour a
// database's network scope. *store.DB satisfies it.
type DatabaseScopeStore interface {
	GetDatabaseAccessSettings(ctx context.Context, database string) (store.DatabaseAccessSettings, error)
	EnvironmentOfDatabase(ctx context.Context, databaseName string) (*store.EnvironmentRef, error)
}

// WithDatabaseScope makes the controller refuse to attach a service's network
// to a database whose scope excludes it, and detach one that is already
// attached. Without it every referenced database is attached, as before.
func WithDatabaseScope(s DatabaseScopeStore) Option {
	return func(c *Controller) { c.databaseScope = s }
}

// databaseScopeAllows reports whether desired may reach dbName. A database
// with the platform-wide scope, or a controller without a scope store,
// allows everything.
func (c *Controller) databaseScopeAllows(ctx context.Context, desired *store.DesiredService, dbName string) (bool, string, error) {
	if c.databaseScope == nil || c.databases == nil {
		return true, "", nil
	}
	settings, err := c.databaseScope.GetDatabaseAccessSettings(ctx, dbName)
	if err != nil {
		return false, "", fmt.Errorf("read network scope of database %q: %w", dbName, err)
	}
	scope := dbaccess.Scope(settings.Scope)
	if scope == dbaccess.ScopePlatform || !scope.Valid() {
		return true, "", nil
	}
	db, err := c.databases.GetDesiredDatabase(ctx, dbName)
	if err != nil {
		return false, "", fmt.Errorf("get database %q for scope check: %w", dbName, err)
	}
	place := dbaccess.Placement{Name: dbName, ProjectID: db.ProjectID}
	if scope == dbaccess.ScopeEnvironment {
		ref, err := c.databaseScope.EnvironmentOfDatabase(ctx, dbName)
		if err != nil {
			return false, "", fmt.Errorf("read environment of database %q: %w", dbName, err)
		}
		if ref != nil {
			place.EnvironmentID = ref.ID
		}
	}
	ok, why := dbaccess.Allows(scope, place, dbaccess.Placement{
		Name: desired.Name, ProjectID: desired.ProjectID, EnvironmentID: desired.EnvironmentID,
	})
	return ok, why, nil
}

// enforceDatabaseScope detaches an out-of-scope service's network from the
// database container and reports the violation, so the condition says why
// the service cannot reach it instead of the connection failing silently.
func (c *Controller) enforceDatabaseScope(ctx context.Context, networkName, dbName, reason string) error {
	containerName := database.ContainerName(dbName)
	state, err := c.runtime.InspectByName(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect database %q before detaching: %w", dbName, err)
	}
	if state != nil {
		if err := c.runtime.NetworkDisconnect(ctx, networkName, containerName, true); err != nil {
			return fmt.Errorf("detach database %q from network %q: %w", dbName, networkName, err)
		}
	}
	return fmt.Errorf("%w: %s (change the database's network scope or move the app)", ErrDatabaseOutOfScope, reason)
}
