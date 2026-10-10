package datamigrate

import (
	"fmt"
	"strconv"
)

// Source credentials reach the helper container as environment variables
// only, never inside a command line, so they stay out of exec arguments.
const (
	envHost     = "SRC_HOST"
	envPort     = "SRC_PORT"
	envUser     = "SRC_USER"
	envPassword = "SRC_PASSWORD"
	envDatabase = "SRC_DB"
	envAuthDB   = "SRC_AUTHDB"
	envTLS      = "SRC_TLS"
)

const sqlBacktick = "\\`"

// pgReadOnly makes every Postgres session the helper opens refuse writes.
const pgReadOnly = `export PGOPTIONS="-c default_transaction_read_only=on"`

// helperEnv is the environment of the helper container that reads the source.
func helperEnv(s Source) map[string]string {
	env := map[string]string{
		envHost:     s.Host,
		envPort:     strconv.Itoa(s.Port),
		envUser:     s.User,
		envPassword: s.Password,
		envDatabase: s.Database,
		envAuthDB:   s.AuthDatabase,
	}
	if s.TLS {
		env[envTLS] = "1"
	}
	return env
}

// pgCountQuery lists every base table with its exact row count.
const pgCountQuery = `SELECT table_schema || '.' || table_name, (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %I.%I', table_schema, table_name), false, true, '')))[1]::text FROM information_schema.tables WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY 1;`

const mongoCountJS = `var only = (typeof process !== "undefined" && process.env.SRC_DB) || ""; db.getMongo().getDBNames().forEach(function (n) { if (n === "admin" || n === "local" || n === "config") { return; } if (only !== "" && n !== only) { return; } var d = db.getSiblingDB(n); d.getCollectionNames().forEach(function (c) { print(n + "." + c + "|" + d.getCollection(c).countDocuments({})); }); });`

// DumpCommand streams a logical dump of the source on stdout. It runs in the
// helper container, where the SRC_* variables are set.
func DumpCommand(engine string) ([]string, error) {
	var script string
	switch engine {
	case EnginePostgres:
		script = `export PGHOST="$SRC_HOST" PGPORT="$SRC_PORT" PGUSER="$SRC_USER" PGPASSWORD="$SRC_PASSWORD" PGDATABASE="$SRC_DB" PGCONNECT_TIMEOUT=20
` + pgReadOnly + `
if [ -n "$SRC_TLS" ]; then export PGSSLMODE=require; fi
exec pg_dump --no-password --no-owner --no-privileges`
	case EngineMySQL:
		script = `export MYSQL_PWD="$SRC_PASSWORD"
ssl=""; if [ -n "$SRC_TLS" ]; then ssl="--ssl-mode=REQUIRED"; fi
exec mysqldump --protocol=tcp -h"$SRC_HOST" -P"$SRC_PORT" -u"$SRC_USER" $ssl --single-transaction --routines --no-tablespaces --column-statistics=0 "$SRC_DB"`
	case EngineMariaDB:
		script = `export MYSQL_PWD="$SRC_PASSWORD"
ssl=""; if [ -n "$SRC_TLS" ]; then ssl="--ssl"; fi
exec mariadb-dump --protocol=tcp -h"$SRC_HOST" -P"$SRC_PORT" -u"$SRC_USER" $ssl --single-transaction --routines --no-tablespaces "$SRC_DB"`
	case EngineMongoDB:
		script = `set -- --host "$SRC_HOST" --port "$SRC_PORT" --authenticationDatabase "${SRC_AUTHDB:-admin}"
if [ -n "$SRC_USER" ]; then set -- "$@" --username "$SRC_USER" --password "$SRC_PASSWORD"; fi
if [ -n "$SRC_DB" ]; then set -- "$@" --db "$SRC_DB"; fi
if [ -n "$SRC_TLS" ]; then set -- "$@" --tls; fi
exec mongodump --archive "$@"`
	case EngineRedis:
		script = `if [ -n "$SRC_PASSWORD" ]; then export REDISCLI_AUTH="$SRC_PASSWORD"; fi
set -- -h "$SRC_HOST" -p "$SRC_PORT"
if [ -n "$SRC_USER" ]; then set -- "$@" --user "$SRC_USER"; fi
if [ -n "$SRC_TLS" ]; then set -- "$@" --tls; fi
exec redis-cli "$@" --rdb -`
	default:
		return nil, fmt.Errorf("copying data is not supported for engine %q", engine)
	}
	return []string{"sh", "-c", script}, nil
}

// SourceCountCommand prints "name|count" lines for the source, run in the helper.
func SourceCountCommand(engine string) ([]string, error) {
	var script string
	switch engine {
	case EnginePostgres:
		script = `export PGHOST="$SRC_HOST" PGPORT="$SRC_PORT" PGUSER="$SRC_USER" PGPASSWORD="$SRC_PASSWORD" PGDATABASE="$SRC_DB" PGCONNECT_TIMEOUT=20
` + pgReadOnly + `
if [ -n "$SRC_TLS" ]; then export PGSSLMODE=require; fi
psql --no-password -At -F '|' -v ON_ERROR_STOP=1 <<'SQL'
` + pgCountQuery + `
SQL`
	case EngineMySQL:
		script = mysqlCountScript(`export MYSQL_PWD="$SRC_PASSWORD"
ssl=""; if [ -n "$SRC_TLS" ]; then ssl="--ssl-mode=REQUIRED"; fi
q() { mysql --protocol=tcp -h"$SRC_HOST" -P"$SRC_PORT" -u"$SRC_USER" $ssl -N -B "$@" "$SRC_DB"; }`)
	case EngineMariaDB:
		script = mysqlCountScript(`export MYSQL_PWD="$SRC_PASSWORD"
ssl=""; if [ -n "$SRC_TLS" ]; then ssl="--ssl"; fi
q() { mariadb --protocol=tcp -h"$SRC_HOST" -P"$SRC_PORT" -u"$SRC_USER" $ssl -N -B "$@" "$SRC_DB"; }`)
	case EngineMongoDB:
		script = `set -- --quiet --host "$SRC_HOST" --port "$SRC_PORT" --authenticationDatabase "${SRC_AUTHDB:-admin}"
if [ -n "$SRC_USER" ]; then set -- "$@" --username "$SRC_USER" --password "$SRC_PASSWORD"; fi
if [ -n "$SRC_TLS" ]; then set -- "$@" --tls; fi
exec mongosh "$@" --eval '` + mongoCountJS + `'`
	case EngineRedis:
		script = `if [ -n "$SRC_PASSWORD" ]; then export REDISCLI_AUTH="$SRC_PASSWORD"; fi
set -- -h "$SRC_HOST" -p "$SRC_PORT"
if [ -n "$SRC_USER" ]; then set -- "$@" --user "$SRC_USER"; fi
if [ -n "$SRC_TLS" ]; then set -- "$@" --tls; fi
n=$(redis-cli "$@" DBSIZE) || exit 1
printf 'keys|%s\n' "$n"`
	default:
		return nil, fmt.Errorf("copying data is not supported for engine %q", engine)
	}
	return []string{"sh", "-c", script}, nil
}

// TargetCountCommand is SourceCountCommand for the managed database, run in its
// own container where the engine's own credential variables are already set.
func TargetCountCommand(engine string) ([]string, error) {
	var script string
	switch engine {
	case EnginePostgres:
		script = `psql --no-password -At -F '|' -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$POSTGRES_USER" <<'SQL'
` + pgCountQuery + `
SQL`
	case EngineMySQL:
		script = mysqlCountScript(`q() { mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -N -B "$@" "$MYSQL_DATABASE"; }`)
	case EngineMariaDB:
		script = mysqlCountScript(`q() { mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -N -B "$@" "$MARIADB_DATABASE"; }`)
	case EngineMongoDB:
		script = `exec mongosh --quiet --host 127.0.0.1 --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval '` + mongoCountJS + `'`
	case EngineRedis:
		script = `RTLS=""; if [ -f /certs/tls.crt ]; then RTLS="--tls --insecure -p 6380"; fi
n=$(redis-cli $RTLS DBSIZE) || exit 1
printf 'keys|%s\n' "$n"`
	default:
		return nil, fmt.Errorf("copying data is not supported for engine %q", engine)
	}
	return []string{"sh", "-c", script}, nil
}

// mysqlCountScript defines q (a mysql client bound to the right server) via
// prelude, then prints "table|count" for every base table.
func mysqlCountScript(prelude string) string {
	return prelude + `
tables=$(q -e 'SHOW FULL TABLES WHERE Table_type = "BASE TABLE"') || exit 1
printf '%s\n' "$tables" | cut -f1 | while IFS= read -r t; do
  [ -n "$t" ] || continue
  n=$(q -e "SELECT COUNT(*) FROM ` + sqlBacktick + `$t` + sqlBacktick + `") || exit 1
  printf '%s|%s\n' "$t" "$n"
done`
}
