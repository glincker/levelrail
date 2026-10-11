package dbupgrade

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// redisTLS mirrors internal/backup's probe: use the TLS port when certs are mounted.
const redisTLS = `RTLS=""; if [ -f /certs/tls.crt ]; then RTLS="--tls --insecure -p 6380"; fi; `

// HealthCommand returns the in-container check for engine. It connects,
// runs a trivial query, checks the server is not a read-only replica where
// that applies, and prints the server version on its first line. Each
// check exits non-zero on failure.
func HealthCommand(engine string) ([]string, error) {
	var script string
	switch engine {
	case store.EnginePostgres:
		script = `out=$(psql --no-password -v ON_ERROR_STOP=1 -Atq -U "$POSTGRES_USER" "$POSTGRES_USER" -c 'SHOW server_version' -c 'SELECT 1' -c 'SELECT pg_is_in_recovery()') || exit 1
[ "$(printf '%s\n' "$out" | sed -n 2p)" = "1" ] || { echo "SELECT 1 failed"; exit 1; }
[ "$(printf '%s\n' "$out" | sed -n 3p)" = "f" ] || { echo "server is in recovery"; exit 1; }
printf '%s\n' "$out" | sed -n 1p`
	case store.EngineMySQL:
		script = `exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -N -B -e 'SELECT VERSION(); SELECT 1'`
	case store.EngineMariaDB:
		script = `exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -N -B -e 'SELECT VERSION(); SELECT 1'`
	case store.EngineMongoDB:
		script = `exec mongosh --quiet --host 127.0.0.1 --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'if (db.adminCommand({ping: 1}).ok !== 1) { quit(1) }; print(db.version())'`
	case store.EngineRedis, store.EngineKeyDB, store.EngineDragonfly:
		script = redisTLS + `info=$(redis-cli $RTLS INFO | tr -d '\r') || exit 1
[ "$(redis-cli $RTLS PING | tr -d '\r')" = "PONG" ] || { echo "PING failed"; exit 1; }
role=$(printf '%s\n' "$info" | sed -n 's/^role://p')
[ "$role" = "master" ] || { echo "role is $role"; exit 1; }
v=$(printf '%s\n' "$info" | sed -n 's/^dragonfly_version:df-//p')
[ -n "$v" ] || v=$(printf '%s\n' "$info" | sed -n 's/^redis_version://p')
echo "$v"`
	case store.EngineClickHouse:
		script = `exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query 'SELECT version()'`
	default:
		return nil, fmt.Errorf("no health check for engine %q", engine)
	}
	return []string{"sh", "-c", script}, nil
}

// parseHealthVersion returns the first non-empty output line.
func parseHealthVersion(out string) string {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			return line
		}
	}
	return ""
}
