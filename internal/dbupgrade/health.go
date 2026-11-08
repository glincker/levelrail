package dbupgrade

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// redisTLS mirrors internal/backup's probe: use the TLS port when certs are mounted.
// The KeyDB image ships keydb-cli instead of redis-cli, so pick whichever exists and fail closed if neither does.
const redisTLS = `RTLS=""; if [ -f /certs/tls.crt ]; then RTLS="--tls --insecure -p 6380"; fi; ` +
	`RCLI=$(command -v redis-cli || command -v keydb-cli) || { echo "no redis-compatible client in image"; exit 1; }; `

// dragonflyHealthScript speaks RESP over bash /dev/tcp because the image ships
// no client binary. Dragonfly runs plaintext here (no TLS), so this port is enough.
const dragonflyHealthScript = `exec 3<>/dev/tcp/127.0.0.1/6379 || exit 1
printf 'PING\r\nINFO\r\nQUIT\r\n' >&3
out=$(cat <&3 | tr -d '\r')
printf '%s\n' "$out" | grep -q '^+PONG$' || { echo "PING failed"; exit 1; }
role=$(printf '%s\n' "$out" | sed -n 's/^role://p' | sed -n 1p)
[ "$role" = "master" ] || { echo "role is $role"; exit 1; }
v=$(printf '%s\n' "$out" | sed -n 's/^dragonfly_version:df-//p')
[ -n "$v" ] || v=$(printf '%s\n' "$out" | sed -n 's/^redis_version://p')
[ -n "$v" ] || { echo "no version in INFO"; exit 1; }
echo "$v"`

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
	case store.EngineDragonfly:
		return []string{"bash", "-c", dragonflyHealthScript}, nil
	case store.EngineRedis, store.EngineKeyDB, store.EngineValkey:
		script = redisTLS + `info=$("$RCLI" $RTLS INFO | tr -d '\r') || exit 1
[ "$("$RCLI" $RTLS PING | tr -d '\r')" = "PONG" ] || { echo "PING failed"; exit 1; }
role=$(printf '%s\n' "$info" | sed -n 's/^role://p')
[ "$role" = "master" ] || { echo "role is $role"; exit 1; }
v=$(printf '%s\n' "$info" | sed -n 's/^redis_version://p')
echo "$v"`
	case store.EngineClickHouse:
		script = `exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query 'SELECT version()'`
	case store.EngineMeilisearch, store.EngineQdrant, store.EngineTypesense:
		return searchHealthCommand(engine), nil
	case store.EngineMemcached:
		return []string{"bash", "-c", `exec 3<>/dev/tcp/127.0.0.1/11211 || exit 1
printf 'stats\r\nquit\r\n' >&3
out=$(cat <&3 | tr -d '\r')
v=$(printf '%s\n' "$out" | sed -n 's/^STAT version //p')
[ -n "$v" ] || { echo "no version in stats"; exit 1; }
echo "$v"`}, nil
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
