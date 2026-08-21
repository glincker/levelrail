package database

import "github.com/GLINCKER/levelrail/internal/store"

// PostgresPasswordEnvKey, MySQLPasswordEnvKey, MongoPasswordEnvKey are the
// internal/secrets envKeys each engine's generated password is stored
// under, keyed by the database's own name as the secrets manager's
// "serviceName" (cmd/levelrail's postgresCredentialsFor and its two
// siblings). Exported and centralized here, not left as unexported
// consts in cmd/levelrail, so internal/deploy and
// internal/reconcile/application can resolve a { from: ... } env var's
// password against the exact same keys those credential generators
// already write, with no second copy of the string literals to drift.
const (
	PostgresPasswordEnvKey   = "postgres_password"
	MySQLPasswordEnvKey      = "mysql_password"
	MongoPasswordEnvKey      = "mongo_password"
	MariaDBPasswordEnvKey    = "mariadb_password"
	ClickHousePasswordEnvKey = "clickhouse_password"
)

// ContainerName exports containerName: the Docker container name dbName's
// managed database reconciles to, also the host part of a resolved
// { from: ... } env var (internal/reconcile/application's
// resolveDatabaseField).
func ContainerName(dbName string) string { return containerName(dbName) }

// ContainerPort returns the standard container port engine's image
// listens on, and whether engine is recognized.
func ContainerPort(engine string) (int, bool) {
	switch engine {
	case store.EnginePostgres:
		return postgresContainerPort, true
	case store.EngineRedis:
		return redisContainerPort, true
	case store.EngineMySQL:
		return mysqlContainerPort, true
	case store.EngineMongoDB:
		return mongoContainerPort, true
	case store.EngineMariaDB:
		// MariaDB reuses the mysql image's port, same reason
		// controller.go's own MariaDB case reuses mysqlContainerPort.
		return mysqlContainerPort, true
	case store.EngineKeyDB:
		// KeyDB reuses the redis image's port, same reason
		// controller.go's own KeyDB case reuses redisContainerPort.
		return redisContainerPort, true
	case store.EngineDragonfly:
		// Dragonfly reuses the redis image's port, same reason
		// controller.go's own Dragonfly case reuses redisContainerPort.
		return redisContainerPort, true
	case store.EngineClickHouse:
		return clickhouseContainerPort, true
	default:
		return 0, false
	}
}

// PasswordSecretKey returns the internal/secrets envKey engine's
// generated password is stored under, and whether engine has a password
// at all: Redis runs passwordless (this package's own doc comment on
// why), so it reports ok=false rather than an empty key.
func PasswordSecretKey(engine string) (key string, ok bool) {
	switch engine {
	case store.EnginePostgres:
		return PostgresPasswordEnvKey, true
	case store.EngineMySQL:
		return MySQLPasswordEnvKey, true
	case store.EngineMongoDB:
		return MongoPasswordEnvKey, true
	case store.EngineMariaDB:
		return MariaDBPasswordEnvKey, true
	case store.EngineClickHouse:
		return ClickHousePasswordEnvKey, true
	default:
		return "", false
	}
}

// SupportsField reports whether field is resolvable for a database of
// engine. The single source of truth internal/deploy.Pipeline.validateEnv
// (deploy-time) and internal/reconcile/application's resolveDatabaseField
// (reconcile-time) both defer to, so which fields work for which engine
// can never drift between the two.
func SupportsField(engine, field string) bool {
	switch field {
	case "url", "host", "port":
		_, ok := ContainerPort(engine)
		return ok
	case "username", "database":
		if engine == store.EngineRedis || engine == store.EngineKeyDB || engine == store.EngineDragonfly {
			return false
		}
		_, ok := ContainerPort(engine)
		return ok
	case "password":
		_, ok := PasswordSecretKey(engine)
		return ok
	default:
		return false
	}
}
