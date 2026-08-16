package backup

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// redisStopTimeout bounds how long Stop waits for Redis to exit cleanly
// during a restore before forcing it, the same shutdown window a rolling
// deploy would give any container.
const redisStopTimeout = 10 * time.Second

// Restorer applies a previously taken dump back onto a live database
// container, the exact reverse of Dumper: instead of producing a stream
// from a running container, it consumes one and feeds it in. Like
// Dumper, it takes no credentials: Postgres and MySQL's restore commands
// authenticate the same way their dump counterparts do, off the
// container's own inherited environment (see dump.go's own doc comment
// on postgresDumpCmd/mysqldumpCmd for why that's correct and sufficient).
//
// The returned error is nil only if the restore command itself exited
// zero; a Restorer implementation is not expected to verify the
// database's contents afterward, the same "ran successfully" contract
// Dumper's own Dump method carries, not "and I checked the result is
// correct."
type Restorer interface {
	Restore(ctx context.Context, engine, containerName string, dump io.Reader) error
}

// ContainerRestorer is the real Restorer: it runs each engine's own
// native restore path inside the already-running database container via
// docker.Runtime.ExecWithInput, mirroring ContainerDumper's own reasoning
// for running dump tools inside the container rather than connecting to
// it from outside (see dump.go's ContainerDumper doc comment; every word
// of that applies here too, restore just needs a stdin stream in
// addition to a stdout one, which is exactly what ExecWithInput adds
// over Exec).
type ContainerRestorer struct {
	Runtime docker.Runtime
}

// Restore implements Restorer.
func (r *ContainerRestorer) Restore(ctx context.Context, engine, containerName string, dump io.Reader) error {
	if engine == store.EngineRedis {
		return r.restoreRedis(ctx, containerName, dump)
	}

	var cmd []string
	switch engine {
	case store.EnginePostgres:
		cmd = postgresRestoreCmd
	case store.EngineMySQL:
		cmd = mysqlRestoreCmd
	default:
		// Mirrors ContainerDumper.Dump's own reasoning for its identical
		// default branch: a genuinely unknown engine reaching here means
		// the caller skipped internal/reconcile/database's own engine
		// validation, not a real gap in restore coverage.
		return fmt.Errorf("backup: restore: unrecognized engine %q", engine)
	}

	rc, err := r.Runtime.ExecWithInput(ctx, containerName, cmd, dump)
	if err != nil {
		return fmt.Errorf("backup: restore %s container %q: %w", engine, containerName, err)
	}
	defer func() {
		_ = rc.Close()
	}()
	// Restore commands (psql, mysql) write ordinary status/notice text to
	// stdout as they go (e.g. psql's own "CREATE TABLE", "COPY 42"
	// progress lines for every statement it runs); this is not the
	// output backup uses at all the way pg_dump's stdout is the entire
	// point for a Dumper, it just has to be drained so the exec session
	// can reach EOF and, if the command exited non-zero, so the real
	// error (assembled from stderr by docker.Client's own
	// streamExecOutput, the same machinery Exec already relies on) is the
	// one that surfaces here instead of a partially-read stream.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("backup: restore %s container %q: %w", engine, containerName, err)
	}
	return nil
}

// restoreRedis loads an RDB snapshot the only way Redis actually supports
// one: write it to disk as /data/dump.rdb while the container is still
// running, then stop and start the container so Redis loads it during its
// own startup sequence, the unconditional-at-boot RDB load that applies
// whenever AOF isn't configured (see redisDataPath's own callers in
// internal/reconcile/database.Controller, which never set one).
//
// Before writing, this clears Redis's own save points for the running
// session (CONFIG SET save "", not persisted to disk, reverts on the
// restart below). Without this, Redis's own SIGTERM handler performs its
// configured snapshot on the way down whenever any save point exists
// (true by default for this image), overwriting the dump.rdb this method
// just wrote with the live pre-restore state a split second before the
// process actually exits: a real failure mode caught by
// TestContainerRestorer_Restore_Redis_Live, not a hypothetical one.
func (r *ContainerRestorer) restoreRedis(ctx context.Context, containerName string, dump io.Reader) error {
	if err := r.drainExec(ctx, containerName, redisDisableSaveCmd); err != nil {
		return fmt.Errorf("backup: restore redis container %q: disable save points: %w", containerName, err)
	}

	rc, err := r.Runtime.ExecWithInput(ctx, containerName, redisRestoreWriteCmd, dump)
	if err != nil {
		return fmt.Errorf("backup: restore redis container %q: write dump.rdb: %w", containerName, err)
	}
	writeErr := func() error {
		defer func() { _ = rc.Close() }()
		_, err := io.Copy(io.Discard, rc)
		return err
	}()
	if writeErr != nil {
		return fmt.Errorf("backup: restore redis container %q: write dump.rdb: %w", containerName, writeErr)
	}

	state, err := r.Runtime.InspectByName(ctx, containerName)
	if err != nil {
		return fmt.Errorf("backup: restore redis container %q: inspect: %w", containerName, err)
	}
	if state == nil {
		return fmt.Errorf("backup: restore redis container %q: not found", containerName)
	}

	if err := r.Runtime.Stop(ctx, state.ID, redisStopTimeout); err != nil {
		return fmt.Errorf("backup: restore redis container %q: stop: %w", containerName, err)
	}
	if err := r.Runtime.Start(ctx, state.ID); err != nil {
		return fmt.Errorf("backup: restore redis container %q: start: %w", containerName, err)
	}
	return nil
}

// redisRestoreWriteCmd streams ExecWithInput's stdin straight onto disk
// as the RDB file Redis loads at its next startup, the write half of
// restoreRedis's write-then-stop-then-start sequence.
var redisRestoreWriteCmd = []string{"sh", "-c", "cat > /data/dump.rdb"}

// redisDisableSaveCmd clears every configured save point for the
// current session only, see restoreRedis's own doc comment for why.
var redisDisableSaveCmd = []string{"redis-cli", "CONFIG", "SET", "save", ""}

// drainExec runs cmd via Exec and discards its output, surfacing only
// whether it failed: used for restoreRedis steps that don't need the
// command's own stdout, mirroring Restore's own drain-then-check
// handling of psql/mysql's status output above.
func (r *ContainerRestorer) drainExec(ctx context.Context, containerName string, cmd []string) error {
	rc, err := r.Runtime.Exec(ctx, containerName, cmd)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

// postgresRestoreCmd is pg_dump's plain-SQL output (see dump.go's own
// postgresDumpCmd comment on why --format=custom was never used) piped
// into psql, the only tool a plain SQL dump needs to be restored: no
// pg_restore, which is for pg_dump's --format=custom/directory/tar
// outputs only and would reject this dump's format outright.
//
// A deliberate drop-and-replace decision, not a merge: before reading
// the dump from stdin, this first drops and recreates the public schema
// (DROP SCHEMA public CASCADE; CREATE SCHEMA public;), the same effect
// as dropping and recreating the target database itself without
// needing a second connection to a different maintenance database to do
// it (the exec session is already connected to the one database this
// restore targets, via $POSTGRES_USER exactly as postgresDumpCmd
// connects, and Postgres does not allow dropping the database a session
// is currently connected to). "Restore" means "replace this database's
// contents with the backup's," not "attempt to layer the backup's rows
// on top of whatever is already there and hope nothing collides on a
// primary key," the same reasoning a merge-shaped restore would get
// wrong on any table that already has rows. This assumes every object
// pg_dump captured lived in the public schema, true for every database
// this controller creates (internal/reconcile/database.Controller never
// creates additional schemas for a managed Postgres database), not a
// general-purpose assumption about arbitrary Postgres databases.
//
// Run through sh -c, the same reasoning postgresDumpCmd's own doc
// comment gives: the shell expands $POSTGRES_USER against the exec'd
// process's own inherited environment. The final exec psql (not a plain
// psql) matters here in a way it didn't for the single-command dump: the
// shell has to stay alive through the first psql -c statement, so the
// leading command deliberately does not use exec, only the trailing one
// that inherits the exec session's stdin does; if the first command were
// exec'd instead, the shell (and with it, the ability to run a second
// command afterward) would be replaced before the schema reset ever ran.
var postgresRestoreCmd = []string{"sh", "-c", `psql --no-password -U "$POSTGRES_USER" "$POSTGRES_USER" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" && exec psql --no-password -U "$POSTGRES_USER" "$POSTGRES_USER"`}

// mysqlRestoreCmd drops and recreates $MYSQL_DATABASE before piping
// mysqldumpCmd's own output into mysql, the same full drop-and-replace
// outcome postgresRestoreCmd achieves via an explicit schema reset, and
// for the same reason: "restore" has to mean "this database's contents
// are now exactly the backup's," not "every table the backup happened to
// mention got overwritten, but anything created since the backup was
// taken is still sitting there."
//
// An earlier version of this command relied on mysqldumpCmd's own
// output alone, on the reasoning that mysqldumpCmd (dump.go) runs with
// none of --no-create-info/--skip-add-drop-table, so mysqldump's default
// behavior emits a "DROP TABLE IF EXISTS `t`;" immediately before every
// "CREATE TABLE `t`" in the dump. That reasoning was correct as far as it
// went, every table the dump actually mentions really does get dropped
// and recreated, but it missed the case a live round-trip test (see
// restore_live_test.go's TestContainerRestorer_Restore_MySQL_Live, which
// caught this directly rather than it being caught on paper) exposed: a
// table created after the backup was taken, never mentioned in the dump
// at all, has no DROP TABLE IF EXISTS for mysqldump to have emitted, so
// it survives a restore untouched. Postgres's schema-drop approach never
// had this gap, since it resets the entire schema before restoring
// regardless of what the dump does or doesn't mention; this command now
// matches that by resetting the whole database up front instead of
// trusting the dump's own per-table statements to cover every case.
//
// Run through sh -c for the same two-statement, don't-exec-the-first-one
// reasoning postgresRestoreCmd's own doc comment gives: the leading mysql
// invocation (a plain -e statement, no stdin involved) has to finish and
// let the shell continue on to the second, stdin-reading mysql
// invocation, so only the trailing command is exec'd.
var mysqlRestoreCmd = []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS $MYSQL_DATABASE; CREATE DATABASE $MYSQL_DATABASE;" && exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`}
