package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ContainerDumper is the real Dumper: it runs each engine's own native
// dump tool inside the already-running database container via
// docker.Runtime.Exec, so this process never needs its own copy of
// pg_dump/mysqldump/etc. and the database never needs a network port
// exposed anywhere reachable.
type ContainerDumper struct {
	Runtime docker.Runtime
}

// Dump implements Dumper.
func (d *ContainerDumper) Dump(ctx context.Context, engine, containerName string) (io.ReadCloser, error) {
	var cmd []string
	switch engine {
	case store.EnginePostgres:
		cmd = postgresDumpCmd
	case store.EngineMySQL:
		cmd = mysqlDumpCmd
	case store.EngineRedis:
		cmd = redisDumpCmd
	case store.EngineMongoDB:
		cmd = mongoDumpCmd
	case store.EngineMariaDB:
		cmd = mariadbDumpCmd
	case store.EngineKeyDB:
		cmd = keydbDumpCmd
	case store.EngineDragonfly:
		cmd = dragonflyDumpCmd
	case store.EngineClickHouse:
		cmd = clickhouseDumpCmd
	default:
		// A genuinely unknown engine, not a known-but-unsupported one: this
		// is the complete list internal/reconcile/database's controller
		// itself recognizes, so reaching this branch means engine came from
		// somewhere that skipped that validation.
		return nil, fmt.Errorf("backup: dump: unrecognized engine %q", engine)
	}

	rc, err := d.Runtime.Exec(ctx, containerName, cmd)
	if err != nil {
		return nil, fmt.Errorf("backup: dump %s container %q: %w", engine, containerName, err)
	}
	return rc, nil
}

// postgresDumpCmd authenticates off $POSTGRES_USER alone: the controller
// never sets POSTGRES_DB, so the official image defaults the database
// name to the role name, and a docker exec session is always a local
// (trust-authenticated) connection so no password is needed. Run via
// "exec" so the shell expands $POSTGRES_USER and then gets replaced by
// pg_dump itself, not left running as a parent process.
var postgresDumpCmd = []string{"sh", "-c", `exec pg_dump --no-password -U "$POSTGRES_USER" "$POSTGRES_USER"`}

// mysqlDumpCmd authenticates as root using $MYSQL_ROOT_PASSWORD, mandatory
// for the official image to start at all. Scoped to $MYSQL_DATABASE
// rather than --all-databases so this doesn't pull in mysql's own
// internal system schemas.
var mysqlDumpCmd = []string{"sh", "-c", `exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`}

// redisDumpCmd streams a live RDB snapshot via redis-cli's own --rdb -
// flag, which uses Redis's SYNC command internally for a consistent
// snapshot with no separate BGSAVE-then-wait dance. redis-cli writes its
// own progress text to stderr, never stdout, so piping stdout yields only
// the raw RDB bytes. No auth flag: this controller never configures a
// Redis password.
var redisDumpCmd = []string{"redis-cli", "--rdb", "-"}

// mongoDumpCmd authenticates as the root user the official image's
// entrypoint creates from $MONGO_INITDB_ROOT_USERNAME/PASSWORD.
// --archive with no path streams the archive format to stdout instead of
// a directory of BSON files.
var mongoDumpCmd = []string{"sh", "-c", `exec mongodump --archive --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`}

// mariadbDumpCmd is mysqlDumpCmd's MariaDB counterpart: mariadb-dump, not
// mysqldump, because MariaDB 11's official image removes the mysqldump
// binary entirely.
var mariadbDumpCmd = []string{"sh", "-c", `exec mariadb-dump -uroot -p"$MARIADB_ROOT_PASSWORD" "$MARIADB_DATABASE"`}

// keydbDumpCmd is redisDumpCmd's exact counterpart for KeyDB: same --rdb -
// mechanism, keydb-cli because the eqalpha/keydb image ships its own CLI
// binary under that name rather than redis-cli.
var keydbDumpCmd = []string{"keydb-cli", "--rdb", "-"}

// dragonflyDumpCmd cannot reuse redisDumpCmd's --rdb - streaming: unlike
// Redis, Dragonfly's SAVE/BGSAVE default to its own DF snapshot format,
// not RDB, so this forces RDB explicitly via SAVE's own format argument
// (a synchronous, blocking snapshot) to a fixed filename, then reads that
// file back. Dragonfly ships redis-cli for Redis-protocol compatibility.
var dragonflyDumpCmd = []string{"sh", "-c", "redis-cli SAVE RDB dump.rdb && exec cat /data/dump.rdb"}

// clickhouseDumpCmd dumps each table's DDL plus rows as INSERT
// statements rather than native BACKUP/RESTORE, which needs a
// server-side named backup disk this controller never provisions.
var clickhouseDumpCmd = []string{"sh", "-c",
	"set -e\n" +
		"for t in $(clickhouse-client --user \"$CLICKHOUSE_USER\" --password \"$CLICKHOUSE_PASSWORD\" --query \"SHOW TABLES FROM $CLICKHOUSE_DB\"); do\n" +
		"  clickhouse-client --user \"$CLICKHOUSE_USER\" --password \"$CLICKHOUSE_PASSWORD\" --query \"SHOW CREATE TABLE $CLICKHOUSE_DB.$t\" --format TSVRaw\n" +
		"  printf ';\\n'\n" +
		"  clickhouse-client --user \"$CLICKHOUSE_USER\" --password \"$CLICKHOUSE_PASSWORD\" --query \"SELECT * FROM $CLICKHOUSE_DB.$t SETTINGS output_format_sql_insert_table_name = '$t' FORMAT SQLInsert\"\n" +
		"done",
}
