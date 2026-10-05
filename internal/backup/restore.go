package backup

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// redisStopTimeout bounds how long Stop waits for the container to exit
// cleanly during a restore before forcing it.
const redisStopTimeout = 10 * time.Second

// Restorer applies a previously taken dump back onto a live database
// container. Like Dumper, it takes no credentials beyond what the
// container's own environment already provides. Its contract is "the
// restore command exited zero," not "and the data was verified correct
// afterward."
type Restorer interface {
	Restore(ctx context.Context, engine, containerName string, dump io.Reader) error
}

// ContainerRestorer is the real Restorer: it runs each engine's own
// native restore path inside the running container via
// docker.Runtime.ExecWithInput, ContainerDumper's own reasoning plus a
// stdin stream.
type ContainerRestorer struct {
	Runtime docker.Runtime
	// ReadyTimeout/ReadyInterval override the engine readiness wait defaults.
	ReadyTimeout  time.Duration
	ReadyInterval time.Duration
}

const (
	defaultRestoreReadyTimeout  = 3 * time.Minute
	defaultRestoreReadyInterval = 2 * time.Second
)

// readyCmds are TCP-level liveness probes: the official images run a
// socket-only temporary server during first-boot init, so a socket probe
// would pass just before the real server replaces it.
var readyCmds = map[string][]string{
	store.EnginePostgres:   {"sh", "-c", `exec pg_isready -q -h 127.0.0.1 -U "$POSTGRES_USER"`},
	store.EngineMySQL:      {"sh", "-c", `exec mysqladmin ping -h 127.0.0.1 -uroot -p"$MYSQL_ROOT_PASSWORD" --silent`},
	store.EngineMariaDB:    {"sh", "-c", `exec mariadb-admin ping -h 127.0.0.1 -uroot -p"$MARIADB_ROOT_PASSWORD" --silent`},
	store.EngineMongoDB:    {"sh", "-c", `exec mongosh --quiet --host 127.0.0.1 --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'db.adminCommand("ping")'`},
	store.EngineClickHouse: {"sh", "-c", `exec clickhouse-client --host 127.0.0.1 --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "SELECT 1"`},
}

// waitEngineReady blocks until the database accepts TCP connections, so a
// restore into a container that was only just created does not race the
// server's first-boot initialisation.
func (r *ContainerRestorer) waitEngineReady(ctx context.Context, engine, containerName string) error {
	cmd, ok := readyCmds[engine]
	if !ok {
		return nil
	}
	timeout, interval := r.ReadyTimeout, r.ReadyInterval
	if timeout <= 0 {
		timeout = defaultRestoreReadyTimeout
	}
	if interval <= 0 {
		interval = defaultRestoreReadyInterval
	}
	deadline := time.Now().Add(timeout)
	for {
		err := r.drainExec(ctx, containerName, cmd)
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%s in %q not accepting connections after %s: %w", engine, containerName, timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// Restore implements Restorer.
func (r *ContainerRestorer) Restore(ctx context.Context, engine, containerName string, dump io.Reader) error {
	switch engine {
	case store.EngineRedis:
		return r.restoreRedisLike(ctx, containerName, dump, redisCLIBin, redisDisableAutoSaveCmd)
	case store.EngineKeyDB:
		return r.restoreRedisLike(ctx, containerName, dump, keydbCLIBin, keydbDisableAutoSaveCmd)
	case store.EngineDragonfly:
		return r.restoreRedisLike(ctx, containerName, dump, redisCLIBin, dragonflyDisableAutoSaveCmd)
	}

	var cmd []string
	switch engine {
	case store.EnginePostgres:
		cmd = postgresRestoreCmd
	case store.EngineMySQL:
		cmd = mysqlRestoreCmd
	case store.EngineMongoDB:
		cmd = mongoRestoreCmd
	case store.EngineMariaDB:
		cmd = mariadbRestoreCmd
	case store.EngineClickHouse:
		cmd = clickhouseRestoreCmd
	default:
		return fmt.Errorf("backup: restore: unrecognized engine %q", engine)
	}

	if err := r.waitEngineReady(ctx, engine, containerName); err != nil {
		return fmt.Errorf("backup: restore %s container %q: %w", engine, containerName, err)
	}

	rc, err := r.Runtime.ExecWithInput(ctx, containerName, cmd, dump)
	if err != nil {
		return fmt.Errorf("backup: restore %s container %q: %w", engine, containerName, err)
	}
	defer func() {
		_ = rc.Close()
	}()
	// Restore commands write ordinary status text to stdout as they go;
	// this drains it so the exec session reaches EOF and a non-zero exit's
	// real error (assembled from stderr by docker.Client) surfaces here.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("backup: restore %s container %q: %w", engine, containerName, err)
	}
	return nil
}

// redisCLIBin and keydbCLIBin are the two Redis-protocol-compatible CLI
// binary names restoreRedisLike's cliBin parameter accepts. Dragonfly
// also uses redisCLIBin: its own image ships redis-cli, not a separate
// binary.
const (
	redisCLIBin = "redis-cli"
	keydbCLIBin = "keydb-cli"
)

// redisDisableAutoSaveCmd/keydbDisableAutoSaveCmd clear save points so the
// SIGTERM shutdown snapshot doesn't overwrite the restored RDB file a
// moment before the process exits.
var (
	redisDisableAutoSaveCmd = []string{"sh", "-c", redisTLSProbe + ` exec redis-cli $RTLS CONFIG SET save ""`}
	keydbDisableAutoSaveCmd = []string{keydbCLIBin, "CONFIG", "SET", "save", ""}
)

// dragonflyDisableAutoSaveCmd suppresses Dragonfly's shutdown snapshot,
// which is gated by dbfilename rather than a Redis-style save point.
var dragonflyDisableAutoSaveCmd = []string{redisCLIBin, "CONFIG", "SET", "dbfilename", ""}

// restoreRedisLike loads an RDB snapshot the only way Redis-protocol
// engines actually support one: write it to /data/dump.rdb while the
// container is still running, then stop and start the container so it
// loads the file during its own startup sequence.
func (r *ContainerRestorer) restoreRedisLike(ctx context.Context, containerName string, dump io.Reader, cliBin string, disableAutoSaveCmd []string) error {
	if err := r.drainExec(ctx, containerName, disableAutoSaveCmd); err != nil {
		return fmt.Errorf("backup: restore %s container %q: disable auto-save: %w", cliBin, containerName, err)
	}

	rc, err := r.Runtime.ExecWithInput(ctx, containerName, redisRestoreWriteCmd, dump)
	if err != nil {
		return fmt.Errorf("backup: restore %s container %q: write dump.rdb: %w", cliBin, containerName, err)
	}
	writeErr := func() error {
		defer func() { _ = rc.Close() }()
		_, err := io.Copy(io.Discard, rc)
		return err
	}()
	if writeErr != nil {
		return fmt.Errorf("backup: restore %s container %q: write dump.rdb: %w", cliBin, containerName, writeErr)
	}

	state, err := r.Runtime.InspectByName(ctx, containerName)
	if err != nil {
		return fmt.Errorf("backup: restore %s container %q: inspect: %w", cliBin, containerName, err)
	}
	if state == nil {
		return fmt.Errorf("backup: restore %s container %q: not found", cliBin, containerName)
	}

	if err := r.Runtime.Stop(ctx, state.ID, redisStopTimeout); err != nil {
		return fmt.Errorf("backup: restore %s container %q: stop: %w", cliBin, containerName, err)
	}
	if err := r.Runtime.Start(ctx, state.ID); err != nil {
		return fmt.Errorf("backup: restore %s container %q: start: %w", cliBin, containerName, err)
	}
	return nil
}

// redisRestoreWriteCmd streams ExecWithInput's stdin straight onto disk
// as the RDB file loaded at the next startup. Engine-agnostic: a plain
// shell redirect, no CLI binary involved.
var redisRestoreWriteCmd = []string{"sh", "-c", "cat > /data/dump.rdb"}

// drainExec runs cmd via Exec and discards its output, surfacing only
// whether it failed.
func (r *ContainerRestorer) drainExec(ctx context.Context, containerName string, cmd []string) error {
	rc, err := r.Runtime.Exec(ctx, containerName, cmd)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

// postgresRestoreCmd drops and recreates the public schema before piping
// the plain-SQL dump into psql, a full replace rather than a merge: this
// assumes every object pg_dump captured lives in the public schema, true
// for every database this controller creates. Only the trailing psql is
// exec'd, so the shell survives to run the schema reset first.
var postgresRestoreCmd = []string{"sh", "-c", `psql --no-password -U "$POSTGRES_USER" "$POSTGRES_USER" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" && exec psql --no-password -U "$POSTGRES_USER" "$POSTGRES_USER"`}

// shBacktick is an escaped backtick for use inside a double-quoted sh
// string: SQL identifiers are quoted so database names with hyphens work.
const shBacktick = "\\`"

// mysqlRestoreCmd drops and recreates $MYSQL_DATABASE up front rather
// than relying on mysqldump's own per-table DROP TABLE IF EXISTS
// statements, which don't cover a table created after the backup was
// taken (a real gap TestContainerRestorer_Restore_MySQL_Live caught).
var mysqlRestoreCmd = []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS ` + shBacktick + `$MYSQL_DATABASE` + shBacktick + `; CREATE DATABASE ` + shBacktick + `$MYSQL_DATABASE` + shBacktick + `;" && exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`}

// mariadbRestoreCmd is mysqlRestoreCmd's MariaDB counterpart: the mariadb
// client and MARIADB_* env vars, since MariaDB 11's image removes the
// mysql client binary entirely.
var mariadbRestoreCmd = []string{"sh", "-c", `mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS ` + shBacktick + `$MARIADB_DATABASE` + shBacktick + `; CREATE DATABASE ` + shBacktick + `$MARIADB_DATABASE` + shBacktick + `;" && exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" "$MARIADB_DATABASE"`}

// mongoRestoreCmd first drops every non-system database so a collection or
// database created after the backup was taken does not survive restore,
// then feeds mongoDumpCmd's own --archive output into mongorestore --drop.
// --drop alone is not enough: it only drops collections that are present in
// the archive, so anything added post-backup would otherwise pass through
// untouched, which is the exact gap this command closes.
//
// The mongosh eval only ever calls dropDatabase() on names outside
// {admin, config, local}, MongoDB's own reserved system databases. admin
// holds user credentials and roles, config and local hold replication and
// cluster metadata; dropping any of them would break authentication or
// cluster state rather than restore user data, so this exclusion list is
// never optional and must never be narrowed or removed.
var mongoRestoreCmd = []string{"sh", "-c", `mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'db.getMongo().getDBNames().forEach(function(n){if(n!=="admin"&&n!=="local"&&n!=="config"){db.getSiblingDB(n).dropDatabase()}})' && exec mongorestore --archive --drop --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`}

// clickhouseRestoreCmd drops and recreates $CLICKHOUSE_DB (same
// full-replace reasoning as mysqlRestoreCmd), then reconnects with it as
// default so the dump's unqualified INSERT INTO statements resolve.
var clickhouseRestoreCmd = []string{"sh", "-c", `clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "DROP DATABASE IF EXISTS ` + shBacktick + `$CLICKHOUSE_DB` + shBacktick + `; CREATE DATABASE ` + shBacktick + `$CLICKHOUSE_DB` + shBacktick + `" && exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"`}
