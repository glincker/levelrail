package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/extdb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ExternalDatabaseStore is the one read needed to resolve a { from: ... }
// reference to a database this platform does not run. *store.DB satisfies it.
type ExternalDatabaseStore interface {
	GetExternalDatabase(ctx context.Context, name string) (*store.ExternalDatabase, error)
}

// externalStore reports the external database store, which is the same
// object configured through WithDatabaseAttachments when it also implements
// ExternalDatabaseStore (*store.DB does).
func (c *Controller) externalStore() ExternalDatabaseStore {
	s, _ := c.databases.(ExternalDatabaseStore)
	return s
}

// lookupExternal returns the external record for name, or nil when none
// exists or no external store is configured.
func (c *Controller) lookupExternal(ctx context.Context, name string) (*store.ExternalDatabase, error) {
	ext := c.externalStore()
	if ext == nil {
		return nil, nil
	}
	rec, err := ext.GetExternalDatabase(ctx, name)
	if errors.Is(err, store.ErrExternalDatabaseNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get external database %q: %w", name, err)
	}
	return rec, nil
}

func (c *Controller) resolveExternalField(ctx context.Context, rec *store.ExternalDatabase, field string) (string, error) {
	live := extdb.ResolveSource(ctx, c.runtime, extdb.Conn{Host: rec.Host, Network: rec.Network, SourceContainer: rec.SourceContainer})
	ep := extdb.Endpoint{Engine: rec.Engine, Host: live.Host, Port: rec.Port, User: rec.Username, Database: rec.DatabaseName, TLSMode: rec.TLSMode}
	password := ""
	if field == extdb.FieldPassword || field == extdb.FieldURL {
		if c.secretResolver != nil {
			svc := store.ExternalDatabaseSecretsKey(rec.Name)
			exists, err := c.secretResolver.Exists(ctx, svc, store.ExternalDatabasePasswordKey)
			if err != nil {
				return "", fmt.Errorf("check password for external database %q: %w", rec.Name, err)
			}
			if exists {
				if password, err = c.secretResolver.Resolve(ctx, svc, store.ExternalDatabasePasswordKey); err != nil {
					return "", fmt.Errorf("resolve password for external database %q: %w", rec.Name, err)
				}
			}
		}
	}
	return extdb.ResolveField(ep, password, field)
}

// connectExternalNetworks attaches the app container to the Docker network of
// every external database it references. Only the app container is changed;
// the database container is never touched.
func (c *Controller) connectExternalNetworks(ctx context.Context, desired *store.DesiredService, containerName string) error {
	if c.externalStore() == nil {
		return nil
	}
	names := make(map[string]bool, len(desired.DatabaseEnv)+1)
	for _, ref := range desired.DatabaseEnv {
		names[ref.Database] = true
	}
	if att := desired.DatabaseAttachment; att != nil {
		names[att.DatabaseName] = true
	}
	for dbName := range names {
		rec, err := c.lookupExternal(ctx, dbName)
		if err != nil {
			return err
		}
		if rec == nil {
			continue
		}
		network := extdb.ResolveSource(ctx, c.runtime, extdb.Conn{Host: rec.Host, Network: rec.Network, SourceContainer: rec.SourceContainer}).Network
		if network == "" {
			continue
		}
		if err := c.runtime.NetworkConnect(ctx, network, containerName); err != nil {
			return fmt.Errorf("connect %q to network %q of external database %q: %w", containerName, network, dbName, err)
		}
	}
	return nil
}
