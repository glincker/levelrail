package deploy

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	nextCacheCAEnvVar = "LEVELRAIL_NEXT_CACHE_CA"
	nextCacheCAField  = "tls_ca"
)

// NextCacheDatabases is the lookup the CA link needs. A source that also has
// GetDatabaseTLSMode is asked for the TLS intent; otherwise the engine default applies.
type NextCacheDatabases interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
}

type databaseTLSModeGetter interface {
	GetDatabaseTLSMode(ctx context.Context, database string) (string, error)
}

// EnsureNextCacheCALink links LEVELRAIL_NEXT_CACHE_CA to the Redis database's tls_ca
// field when TLS is on and supported, and drops it when next.cache is off or no longer applies.
func EnsureNextCacheCALink(ctx context.Context, desired *store.DesiredService, dbs NextCacheDatabases) {
	url, ok := desired.DatabaseEnv[nextCacheURLEnvVar]
	if desired.NextCache != spec.NextCacheRedis || !ok {
		setNextCacheCA(desired, nil)
		return
	}
	if _, own := desired.Env[nextCacheCAEnvVar]; own {
		return
	}
	if !nextCacheTLSOn(ctx, url.Database, dbs) {
		setNextCacheCA(desired, nil)
		return
	}
	ca := store.DatabaseEnvRef{Database: url.Database, Field: nextCacheCAField}
	setNextCacheCA(desired, &ca)
}

// nextCacheTLSOn is false on any lookup failure so a missing database never adds a link
// the reconciler could not resolve.
func nextCacheTLSOn(ctx context.Context, name string, dbs NextCacheDatabases) bool {
	if dbs == nil || name == "" {
		return false
	}
	db, err := dbs.GetDesiredDatabase(ctx, name)
	if err != nil || db == nil || !database.SupportsField(db.Engine, nextCacheCAField) {
		return false
	}
	mode := store.DatabaseTLSModeDefault
	if g, ok := dbs.(databaseTLSModeGetter); ok {
		if m, err := g.GetDatabaseTLSMode(ctx, name); err == nil {
			mode = m
		}
	}
	return store.DatabaseTLSEffective(db.Engine, mode)
}

// setNextCacheCA copies DatabaseEnv before changing it, since the map may be shared.
func setNextCacheCA(desired *store.DesiredService, ref *store.DatabaseEnvRef) {
	cur, has := desired.DatabaseEnv[nextCacheCAEnvVar]
	if ref == nil && !has || ref != nil && has && cur == *ref {
		return
	}
	next := make(map[string]store.DatabaseEnvRef, len(desired.DatabaseEnv)+1)
	for k, v := range desired.DatabaseEnv {
		if k != nextCacheCAEnvVar {
			next[k] = v
		}
	}
	if ref != nil {
		next[nextCacheCAEnvVar] = *ref
	}
	desired.DatabaseEnv = next
}

// applyNextCacheCA runs after applyNextCache on deploy paths that have the pipeline store.
func (p *Pipeline) applyNextCacheCA(ctx context.Context, req Request, desired *store.DesiredService) {
	if !req.Service.Next.UsesRedisCache() {
		return
	}
	if _, own := req.Service.Env[nextCacheCAEnvVar]; own {
		return
	}
	d := *desired
	d.NextCache = spec.NextCacheRedis
	EnsureNextCacheCALink(ctx, &d, p.store)
	desired.DatabaseEnv = d.DatabaseEnv
}
