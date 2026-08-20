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
	redisDisableAutoSaveCmd = []string{redisCLIBin, "CONFIG", "SET", "save", ""}
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

// mysqlRestoreCmd drops and recreates $MYSQL_DATABASE up front rather
// than relying on mysqldump's own per-table DROP TABLE IF EXISTS
// statements, which don't cover a table created after the backup was
// taken (a real gap TestContainerRestorer_Restore_MySQL_Live caught).
var mysqlRestoreCmd = []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS $MYSQL_DATABASE; CREATE DATABASE $MYSQL_DATABASE;" && exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`}

// mariadbRestoreCmd is mysqlRestoreCmd's MariaDB counterpart: the mariadb
// client and MARIADB_* env vars, since MariaDB 11's image removes the
// mysql client binary entirely.
var mariadbRestoreCmd = []string{"sh", "-c", `mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS $MARIADB_DATABASE; CREATE DATABASE $MARIADB_DATABASE;" && exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" "$MARIADB_DATABASE"`}

// mongoRestoreCmd feeds mongoDumpCmd's own --archive output back in via
// mongorestore --archive --drop, the same full drop-and-replace semantics
// postgresRestoreCmd and mysqlRestoreCmd achieve their own way.
var mongoRestoreCmd = []string{"sh", "-c", `exec mongorestore --archive --drop --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`}

// clickhouseRestoreCmd drops and recreates $CLICKHOUSE_DB (same
// full-replace reasoning as mysqlRestoreCmd), then reconnects with it as
// default so the dump's unqualified INSERT INTO statements resolve.
var clickhouseRestoreCmd = []string{"sh", "-c", `clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "DROP DATABASE IF EXISTS $CLICKHOUSE_DB; CREATE DATABASE $CLICKHOUSE_DB" && exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"`}
