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
// docker.Runtime.Exec (the Engine API's exec facility), never a
// shelled-out `docker exec` subprocess and never a direct network
// connection from this process to the database. Running the dump tool
// inside the container, rather than connecting to it from outside, is
// the only approach that doesn't need this process to also carry a
// pg_dump/mysqldump/redis-cli binary and doesn't need the database's
// port published to a reachable network at all: every managed database
// container already has its own engine's client tools installed, since
// they ship in the same official image as the server.
//
// No credentials are threaded into Dump from the caller, matching the
// Dumper interface's own signature (engine and containerName only). Each
// engine's command below gets what it needs a different way; see runPostgres,
// runMySQL, and runRedis for the specifics, but the short version is:
// Postgres and MySQL commands read their own container's environment
// variables directly (Docker's exec inherits the environment a container
// was created with, documented Engine behavior, not an assumption this
// package is relying on blind), and Redis needs no credential at all
// because internal/reconcile/database's controller never configures one
// for it today.
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
	default:
		// A genuinely unknown engine, not a known-but-unsupported one:
		// store.EnginePostgres/EngineRedis/EngineMySQL/EngineMongoDB/
		// EngineMariaDB/EngineKeyDB is the complete list
		// internal/reconcile/database's controller itself recognizes, so
		// reaching this branch means engine came from somewhere that
		// skipped that validation, not a real gap in dump coverage.
		return nil, fmt.Errorf("backup: dump: unrecognized engine %q", engine)
	}

	rc, err := d.Runtime.Exec(ctx, containerName, cmd)
	if err != nil {
		return nil, fmt.Errorf("backup: dump %s container %q: %w", engine, containerName, err)
	}
	return rc, nil
}

// postgresDumpCmd runs pg_dump inside a Postgres database container,
// authenticating the way this codebase's own database controller
// (internal/reconcile/database's Controller, see WithPostgresCredentials)
// actually configures the container, not a generic Postgres setup:
//
//   - POSTGRES_USER and POSTGRES_PASSWORD are the only credentials env
//     vars the controller sets; POSTGRES_DB is never set. The official
//     Postgres image's own entrypoint defaults the database it creates
//     to POSTGRES_USER's value whenever POSTGRES_DB is absent, so the
//     database this dump needs is always named identically to the role,
//     both held in the single $POSTGRES_USER variable already present in
//     the container's environment. That is why the same expansion,
//     "$POSTGRES_USER", appears as both -U (the role) and the trailing
//     positional argument (the database) below: it is not a copy-paste
//     duplicate, it is the one value this controller's env layout
//     guarantees is correct for both.
//   - The official Postgres image sets up trust authentication for
//     local (Unix socket) connections regardless of
//     POSTGRES_HOST_AUTH_METHOD, which only governs host (TCP)
//     connections; a docker exec session is always a local connection
//     from inside the container, so pg_dump needs no password at all
//     here. --no-password is passed anyway, not because a password is
//     ever required, but so a future image or config change that did
//     require one would make this command fail immediately and loudly
//     (a clear auth error) instead of pg_dump silently blocking on a
//     stdin password prompt that a non-interactive exec session can
//     never satisfy.
//   - No --format=custom: plain SQL output, pg_dump's own default,
//     keeps the resulting object restorable with nothing more than
//     `psql < dump`, matching runner.go's own engine-agnostic ".dump"
//     object naming rather than assuming pg_restore will be available
//     wherever a restore eventually happens.
//
// Run through sh -c so the shell, not this process, expands
// $POSTGRES_USER against the exec'd process's own inherited
// environment; exec (the shell builtin) replaces the shell with pg_dump
// once expansion is done, so the exec'd PID is pg_dump itself, not a
// lingering parent shell.
var postgresDumpCmd = []string{"sh", "-c", `exec pg_dump --no-password -U "$POSTGRES_USER" "$POSTGRES_USER"`}

// mysqlDumpCmd runs mysqldump inside a MySQL database container,
// following the exact pattern the official MySQL image's own
// documentation recommends for backups taken via docker exec: connect
// as root using MYSQL_ROOT_PASSWORD from the container's own inherited
// environment, since that password is mandatory for this image to start
// at all (see internal/reconcile/database.MySQLCredentials' own doc
// comment) and is therefore always present and always correct here,
// unlike POSTGRES_PASSWORD which this dump doesn't even need.
//
// Scoped to $MYSQL_DATABASE rather than --all-databases the way the
// upstream example does: the controller sets MYSQL_DATABASE explicitly
// to the managed database's own name, and dumping "all databases" would
// pull in mysql's internal system schemas along with it, neither wanted
// nor meaningful for a backup of one managed database.
var mysqlDumpCmd = []string{"sh", "-c", `exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"`}

// redisDumpCmd streams a point-in-time RDB snapshot from a Redis
// container using redis-cli's own --rdb flag with "-" as the
// destination, which writes the SYNC transfer's bytes straight to
// stdout instead of a file. This is redis-cli's own documented remote-
// backup mechanism (redis-cli --rdb <file>, "-" as an explicit
// stdout-alias), not a repurposing of some other feature, and it uses
// Redis's SYNC command internally to obtain a fresh, consistent
// snapshot; no separate BGSAVE-then-wait-then-read-the-file dance is
// needed, and there's no race between triggering a save and reading a
// file that may not have finished being written yet.
//
// redis-cli writes its own progress and status text ("SYNC sent to
// master...", "Transfer finished with success.") to stderr, never
// stdout, specifically so that redirecting or piping stdout (exactly
// what Exec's demultiplexing depends on) yields only the raw RDB bytes
// with nothing else mixed in.
//
// No authentication flag: internal/reconcile/database's Controller
// never configures a Redis password today (see that package's own doc
// comment on why an unauthenticated Redis is an accepted posture, unlike
// Postgres or MySQL), so there is no credential for this command to
// supply. If Redis password support is added to that controller later,
// this command needs a matching -a "$REDIS_PASSWORD" (or
// REDISCLI_AUTH) addition; until then, an authenticated Redis container
// would simply reject this command's SYNC, another known, honest
// consequence of the controller's own current scope rather than a gap
// hidden here.
var redisDumpCmd = []string{"redis-cli", "--rdb", "-"}

// mongoDumpCmd runs mongodump inside a MongoDB database container,
// authenticating off the container's own inherited environment the same
// way postgresDumpCmd/mysqlDumpCmd do: MONGO_INITDB_ROOT_USERNAME/
// MONGO_INITDB_ROOT_PASSWORD are the only credentials env vars this
// codebase's own controller sets (internal/reconcile/database's
// Controller, see MongoDBCredentials' own doc comment), and the
// official mongo image's entrypoint uses them to create that root user
// on first boot, exactly what mongodump authenticates as here.
//
// --archive with no path writes the archive format to stdout instead of
// a directory of BSON files, mongodump's own documented mechanism for
// streaming a dump through a pipe (the same reason redis-cli's --rdb -
// exists above): a single self-contained stream this package's Dumper
// interface can hand back as an io.ReadCloser, restorable with
// `mongorestore --archive` on the way back in (see mongoRestoreCmd in
// restore.go), with no intermediate directory either side ever needs to
// materialize.
var mongoDumpCmd = []string{"sh", "-c", `exec mongodump --archive --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`}

// mariadbDumpCmd runs mariadb-dump inside a MariaDB database container,
// authenticating off MARIADB_ROOT_PASSWORD/MARIADB_DATABASE, the
// MariaDB-native env vars this codebase's own controller sets (see
// internal/reconcile/database.MariaDBCredentials' own doc comment), the
// same scoped-to-one-database reasoning mysqlDumpCmd already gives.
//
// mariadb-dump, not mysqldump: from MariaDB 11 the mysqldump binary is
// removed entirely from the official image, so a command that worked
// against MySQL by analogy would fail outright against a modern MariaDB
// container. mariadb-dump is the maintained name going forward.
var mariadbDumpCmd = []string{"sh", "-c", `exec mariadb-dump -uroot -p"$MARIADB_ROOT_PASSWORD" "$MARIADB_DATABASE"`}

// keydbDumpCmd is redisDumpCmd's exact counterpart for KeyDB: same
// --rdb - live-snapshot-to-stdout mechanism, keydb-cli in place of
// redis-cli because the eqalpha/keydb image ships its own CLI binary
// under that name, not a copy of Redis's. No authentication flag for
// the same reason redisDumpCmd has none: this controller never
// configures a KeyDB password either.
var keydbDumpCmd = []string{"keydb-cli", "--rdb", "-"}
