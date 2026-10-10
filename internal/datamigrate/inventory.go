package datamigrate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

// DefaultHelperVersion is the Postgres image tag the read-only helper runs.
// pg_dump needs a client at least as new as the source server.
const DefaultHelperVersion = "17"

const maintenanceDB = "postgres"

// DatabaseInfo is one database found on a source server.
type DatabaseInfo struct {
	Name       string   `json:"name"`
	SizeBytes  int64    `json:"size_bytes"`
	Tables     int      `json:"tables"`
	Extensions []string `json:"extensions,omitempty"`
}

// Inventory is what a read-only look at a source server found.
type Inventory struct {
	Version   string         `json:"version"`
	Major     int            `json:"major"`
	FreeBytes int64          `json:"free_bytes"`
	Databases []DatabaseInfo `json:"databases"`
}

// InventorySupported reports whether a server's databases can be listed.
func InventorySupported(engine string) bool {
	switch engine {
	case EnginePostgres, EngineMySQL, EngineMariaDB, EngineMongoDB:
		return true
	}
	return false
}

const mongoListJS = `print("version|" + db.version()); db.adminCommand({listDatabases: 1}).databases.forEach(function (d) { if (d.name === "admin" || d.name === "local" || d.name === "config") { return; } print("db|" + d.name + "|" + d.sizeOnDisk + "|" + db.getSiblingDB(d.name).getCollectionNames().length); });`

// InventoryListCommand lists the server version and its databases. Postgres
// prints "db|name|bytes"; the others also print a table count.
func InventoryListCommand(engine string) ([]string, error) {
	var script string
	switch engine {
	case EnginePostgres:
		script = `export PGHOST="$SRC_HOST" PGPORT="$SRC_PORT" PGUSER="$SRC_USER" PGPASSWORD="$SRC_PASSWORD" PGDATABASE="${SRC_DB:-` + maintenanceDB + `}" PGCONNECT_TIMEOUT=20
` + pgReadOnly + `
if [ -n "$SRC_TLS" ]; then export PGSSLMODE=require; fi
psql --no-password -At -F '|' -v ON_ERROR_STOP=1 <<'SQL'
SELECT 'version|' || split_part(current_setting('server_version'), ' ', 1);
SELECT 'db|' || datname || '|' || pg_database_size(datname) FROM pg_database WHERE NOT datistemplate AND datallowconn AND has_database_privilege(datname, 'CONNECT') ORDER BY datname;
SQL`
	case EngineMySQL, EngineMariaDB:
		bin, ssl := "mysql", `--ssl-mode=REQUIRED`
		if engine == EngineMariaDB {
			bin, ssl = "mariadb", `--ssl`
		}
		script = `export MYSQL_PWD="$SRC_PASSWORD"
ssl=""; if [ -n "$SRC_TLS" ]; then ssl="` + ssl + `"; fi
exec ` + bin + ` --protocol=tcp -h"$SRC_HOST" -P"$SRC_PORT" -u"$SRC_USER" $ssl --init-command='SET SESSION TRANSACTION READ ONLY' -N -B <<'SQL'
SELECT CONCAT('version|', VERSION());
SELECT CONCAT('db|', table_schema, '|', COALESCE(SUM(data_length + index_length), 0), '|', COUNT(*)) FROM information_schema.tables WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys') GROUP BY table_schema ORDER BY table_schema;
SQL`
	case EngineMongoDB:
		script = `set -- --quiet --host "$SRC_HOST" --port "$SRC_PORT" --authenticationDatabase "${SRC_AUTHDB:-admin}"
if [ -n "$SRC_USER" ]; then set -- "$@" --username "$SRC_USER" --password "$SRC_PASSWORD"; fi
if [ -n "$SRC_TLS" ]; then set -- "$@" --tls; fi
exec mongosh "$@" --eval '` + mongoListJS + `'`
	default:
		return nil, fmt.Errorf("listing databases is not supported for engine %q", engine)
	}
	return []string{"sh", "-c", script}, nil
}

// InventoryDatabaseCommand prints "tables|N" and "ext|name" lines for the
// Postgres database passed as the script's first argument. The name travels as
// a positional argument, so it never reaches a shell or SQL string.
func InventoryDatabaseCommand(dbName string) []string {
	script := `export PGHOST="$SRC_HOST" PGPORT="$SRC_PORT" PGUSER="$SRC_USER" PGPASSWORD="$SRC_PASSWORD" PGDATABASE="$1" PGCONNECT_TIMEOUT=20
` + pgReadOnly + `
if [ -n "$SRC_TLS" ]; then export PGSSLMODE=require; fi
psql --no-password -At -F '|' -v ON_ERROR_STOP=1 <<'SQL'
SELECT 'tables|' || count(*) FROM information_schema.tables WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema');
SELECT 'ext|' || extname FROM pg_extension WHERE extname <> 'plpgsql' ORDER BY extname;
SQL`
	return []string{"sh", "-c", script, "sh", dbName}
}

// DiskFreeCommand prints "free|bytes" for the filesystem the helper runs on,
// the host's Docker storage that the managed database will also write to.
func DiskFreeCommand() []string {
	return []string{"sh", "-c", `df -Pk / | awk 'NR==2 { print "free|" $4 * 1024 }'`}
}

// Inspector reads a source server through a short-lived helper container.
type Inspector struct {
	Runtime       docker.Runtime
	Logger        *slog.Logger
	HelperVersion string
	// HelperNetwork is the Docker network the helper joins, see Copier.
	HelperNetwork string
}

// Inventory lists the databases on the source server. It only runs read-only
// statements, and every error is scrubbed of the source password.
func (i *Inspector) Inventory(ctx context.Context, engine string, src Source) (Inventory, error) {
	inv, err := i.inventory(ctx, engine, src)
	if err != nil {
		return Inventory{}, errors.New(src.Scrub(err.Error()))
	}
	return inv, nil
}

func (i *Inspector) inventory(ctx context.Context, engine string, src Source) (Inventory, error) {
	list, err := InventoryListCommand(engine)
	if err != nil {
		return Inventory{}, err
	}
	version := i.HelperVersion
	if version == "" {
		version = DefaultHelperVersion
	}
	if engine != EnginePostgres {
		version = ""
	}
	helper, err := startHelperContainer(ctx, i.Runtime, database.ImageRef(engine, version), src, "inventory", i.HelperNetwork)
	if err != nil {
		return Inventory{}, err
	}
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if rmErr := i.Runtime.Remove(rmCtx, helper, true); rmErr != nil {
			i.logger().Warn("datamigrate: remove inventory helper failed", slog.String("error", src.Scrub(rmErr.Error())))
		}
	}()

	out, err := execCapture(ctx, i.Runtime, helper, list)
	if err != nil {
		return Inventory{}, fmt.Errorf("list source databases: %w", err)
	}
	inv := ParseInventory(engine, out)
	if engine == EnginePostgres {
		for n := range inv.Databases {
			info := &inv.Databases[n]
			det, err := execCapture(ctx, i.Runtime, helper, InventoryDatabaseCommand(info.Name))
			if err != nil {
				return Inventory{}, fmt.Errorf("inspect source database %q: %w", info.Name, err)
			}
			parseDatabaseDetail(det, info)
		}
	}
	inv.FreeBytes = -1
	if free, err := execCapture(ctx, i.Runtime, helper, DiskFreeCommand()); err == nil {
		inv.FreeBytes = parseKeyed(free, "free")
	}
	return inv, nil
}

func (i *Inspector) logger() *slog.Logger {
	if i.Logger != nil {
		return i.Logger
	}
	return slog.Default()
}

func execCapture(ctx context.Context, rt docker.Runtime, container string, cmd []string) (string, error) {
	rc, err := rt.Exec(ctx, container, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	var sb strings.Builder
	if _, err := io.Copy(&sb, io.LimitReader(rc, 8<<20)); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// ParseInventory reads the output of InventoryListCommand.
func ParseInventory(engine, out string) Inventory {
	var inv Inventory
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "version|"):
			inv.Version = strings.TrimSpace(strings.TrimPrefix(line, "version|"))
			inv.Major = MajorOf(inv.Version)
		case strings.HasPrefix(line, "db|"):
			if info, ok := parseDBLine(engine, strings.TrimPrefix(line, "db|")); ok {
				inv.Databases = append(inv.Databases, info)
			}
		}
	}
	sort.Slice(inv.Databases, func(a, b int) bool { return inv.Databases[a].Name < inv.Databases[b].Name })
	return inv
}

func parseDBLine(engine, rest string) (DatabaseInfo, bool) {
	fields := 2
	if engine != EnginePostgres {
		fields = 3
	}
	parts := rsplit(rest, fields-1)
	if len(parts) != fields || parts[0] == "" {
		return DatabaseInfo{}, false
	}
	size, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || size < 0 {
		size = 0
	}
	info := DatabaseInfo{Name: parts[0], SizeBytes: size}
	if fields == 3 {
		info.Tables, _ = strconv.Atoi(strings.TrimSpace(parts[2]))
	}
	return info, true
}

// rsplit splits s on its last n separators, so a name containing "|" survives.
func rsplit(s string, n int) []string {
	parts := make([]string, 0, n+1)
	for len(parts) < n {
		i := strings.LastIndex(s, "|")
		if i < 0 {
			break
		}
		parts = append([]string{s[i+1:]}, parts...)
		s = s[:i]
	}
	return append([]string{s}, parts...)
}

func parseDatabaseDetail(out string, info *DatabaseInfo) {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "tables|"):
			info.Tables, _ = strconv.Atoi(strings.TrimPrefix(line, "tables|"))
		case strings.HasPrefix(line, "ext|"):
			info.Extensions = append(info.Extensions, strings.TrimPrefix(line, "ext|"))
		}
	}
}

func parseKeyed(out, key string) int64 {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+"|"); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		}
	}
	return -1
}

// MajorOf reads the leading major number of a server version such as "16.4".
func MajorOf(version string) int {
	end := 0
	for end < len(version) && version[end] >= '0' && version[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(version[:end])
	if err != nil {
		return 0
	}
	return n
}
