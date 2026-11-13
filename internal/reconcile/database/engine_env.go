package database

import "github.com/GLINCKER/levelrail/internal/store"

// conventionalEnvPrefix is the variable prefix client libraries of these
// engines read by convention; other engines keep DATABASE_URL naming.
var conventionalEnvPrefix = map[string]string{
	store.EngineMeilisearch: "MEILISEARCH",
	store.EngineQdrant:      "QDRANT",
	store.EngineTypesense:   "TYPESENSE",
	store.EngineMemcached:   "MEMCACHED",
}

// ConventionalEnvVar is the env var name an app conventionally reads field
// of engine from (MEILISEARCH_URL, QDRANT_API_KEY), or "" when engine has
// no such convention or field is not supported for it.
func ConventionalEnvVar(engine, field string) string {
	prefix, ok := conventionalEnvPrefix[engine]
	if !ok && store.IsRedisFamily(engine) {
		prefix, ok = "REDIS", true
		if engine == store.EngineValkey {
			prefix = "VALKEY"
		}
	}
	if !ok || !SupportsField(engine, field) {
		return ""
	}
	switch field {
	case "url":
		if engine == store.EngineMemcached {
			// Memcached clients read a host:port server list, not a URL.
			return prefix + "_SERVERS"
		}
		return prefix + "_URL"
	case "password":
		if store.IsRedisFamily(engine) {
			return prefix + "_PASSWORD"
		}
		return prefix + "_API_KEY"
	case "tls_port":
		return prefix + "_TLS_PORT"
	case "tls_ca", "ca":
		return prefix + "_CA"
	case "host":
		return prefix + "_HOST"
	case "port":
		return prefix + "_PORT"
	}
	return ""
}
