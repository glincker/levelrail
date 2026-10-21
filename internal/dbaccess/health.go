package dbaccess

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrHealthUnsupported means no connection or disk probe exists for the engine.
var ErrHealthUnsupported = fmt.Errorf("database health probe not supported for this engine")

// Scripts run inside the database container and read the root credentials from its own environment, so no secret crosses the exec argv.
func connectionsScript(engine string) (string, bool) {
	switch engine {
	case store.EnginePostgres:
		return `exec psql -X -At -F ' ' --no-password -U "$POSTGRES_USER" "$POSTGRES_USER" ` +
			`-c "SELECT count(*), current_setting('max_connections') FROM pg_stat_activity"`, true
	case store.EngineMySQL:
		return mysqlConnectionsScript("mysql", "MYSQL_ROOT_PASSWORD"), true
	case store.EngineMariaDB:
		return mysqlConnectionsScript("mariadb", "MARIADB_ROOT_PASSWORD"), true
	case store.EngineRedis:
		return `RC="redis-cli"; $RC ping >/dev/null 2>&1 || RC="redis-cli --tls --insecure -p 6380"; ` +
			`u=$($RC INFO clients | tr -d '\r' | awk -F: '/^connected_clients/{print $2}'); ` +
			`m=$($RC CONFIG GET maxclients | tail -1 | tr -d '\r'); echo "$u $m"`, true
	}
	return "", false
}

func mysqlConnectionsScript(bin, passEnv string) string {
	return `P="$` + passEnv + `"; ` +
		`u=$(` + bin + ` -N -B -uroot -p"$P" -e "SHOW GLOBAL STATUS LIKE 'Threads_connected'" 2>/dev/null | awk '{print $2}'); ` +
		`m=$(` + bin + ` -N -B -uroot -p"$P" -e "SELECT @@max_connections" 2>/dev/null); echo "$u $m"`
}

func dataPath(engine string) (string, bool) {
	switch engine {
	case store.EnginePostgres:
		return "/var/lib/postgresql/data", true
	case store.EngineMySQL, store.EngineMariaDB:
		return "/var/lib/mysql", true
	case store.EngineMongoDB:
		return "/data/db", true
	}
	if store.IsRedisFamily(engine) {
		return "/data", true
	}
	return "", false
}

func runHealthScript(ctx context.Context, ex Exec, containerID, script string) (string, error) {
	rc, err := ex.ExecWithInput(ctx, containerID, []string{"sh", "-c", script}, strings.NewReader(""))
	if err != nil {
		return "", wrapExec(err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(io.LimitReader(rc, 4096))
	if err != nil {
		return "", fmt.Errorf("read health probe output: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ReadConnections returns connections in use and the server limit for a running database container.
func ReadConnections(ctx context.Context, ex Exec, containerID, engine string) (used, limit int, err error) {
	script, ok := connectionsScript(engine)
	if !ok {
		return 0, 0, ErrHealthUnsupported
	}
	out, err := runHealthScript(ctx, ex, containerID, script)
	if err != nil {
		return 0, 0, err
	}
	return parseTwoInts(out)
}

// ReadDiskUsedPercent returns how full the filesystem under the database's data directory is.
func ReadDiskUsedPercent(ctx context.Context, ex Exec, containerID, engine string) (float64, error) {
	path, ok := dataPath(engine)
	if !ok {
		return 0, ErrHealthUnsupported
	}
	out, err := runHealthScript(ctx, ex, containerID, `df -P "`+path+`" | awk 'NR==2{gsub("%","",$5);print $5}'`)
	if err != nil {
		return 0, err
	}
	v, perr := strconv.ParseFloat(out, 64)
	if perr != nil {
		return 0, fmt.Errorf("parse disk usage %q: %w", out, perr)
	}
	return v, nil
}

func parseTwoInts(out string) (int, int, error) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected connection probe output %q", out)
	}
	a, errA := strconv.Atoi(fields[0])
	b, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		return 0, 0, fmt.Errorf("unexpected connection probe output %q", out)
	}
	return a, b, nil
}
