package backup

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ComposeServiceEngine maps a compose service image to a backup engine.
// Only Postgres and MySQL/MariaDB are recognized; anything else returns
// false so a non-database service is never offered as a target.
func ComposeServiceEngine(image string) (string, bool) {
	ref := strings.ToLower(strings.TrimSpace(image))
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	// Drop the tag only when the colon is after the last slash (a registry port has none).
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	name := ref
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		name = ref[i+1:]
	}
	switch name {
	case "postgres", "postgis":
		return store.EnginePostgres, true
	case "mysql":
		return store.EngineMySQL, true
	case "mariadb":
		return store.EngineMariaDB, true
	}
	return "", false
}

// composePostgresDumpCmd tolerates a user-chosen POSTGRES_DB and a missing
// POSTGRES_USER, which managed databases never have.
var composePostgresDumpCmd = []string{"sh", "-c",
	`U="${POSTGRES_USER:-postgres}"; exec pg_dump --no-password --no-owner --no-privileges -U "$U" "${POSTGRES_DB:-$U}"`}

// The root password travels in MYSQL_PWD (read from the container's own env),
// never on the command line, so it stays out of process listings and logs.
// Without a named database the whole server is dumped.
var composeMySQLDumpCmd = []string{"sh", "-c",
	`export MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; ` +
		`if [ -n "$MYSQL_DATABASE" ]; then exec mysqldump -uroot --databases "$MYSQL_DATABASE"; fi; ` +
		`exec mysqldump -uroot --all-databases`}

var composeMariaDBDumpCmd = []string{"sh", "-c",
	`export MYSQL_PWD="${MARIADB_ROOT_PASSWORD:-$MYSQL_ROOT_PASSWORD}"; ` +
		`D="${MARIADB_DATABASE:-$MYSQL_DATABASE}"; ` +
		`if [ -n "$D" ]; then exec mariadb-dump -uroot --databases "$D"; fi; ` +
		`exec mariadb-dump -uroot --all-databases`}

// ComposeDumpCmd returns the in-container dump command for a compose
// database service, or an error for an engine not supported here.
func ComposeDumpCmd(engine string) ([]string, error) {
	switch engine {
	case store.EnginePostgres:
		return composePostgresDumpCmd, nil
	case store.EngineMySQL:
		return composeMySQLDumpCmd, nil
	case store.EngineMariaDB:
		return composeMariaDBDumpCmd, nil
	}
	return nil, fmt.Errorf("backup: compose service dump: engine %q is not supported", engine)
}

// ComposeServiceDumper implements Dumper for a database service inside a
// compose app, so the existing storage, encryption and retention path can
// consume it unchanged.
type ComposeServiceDumper struct {
	Runtime docker.Runtime
	// Runtimes picks the runtime for the node carried by ctx; nil falls back to Runtime.
	Runtimes func(nodeID string) (docker.Runtime, error)
}

func composeRuntime(ctx context.Context, fixed docker.Runtime, pick func(string) (docker.Runtime, error)) (docker.Runtime, error) {
	if pick == nil {
		return fixed, nil
	}
	return pick(composeNodeFrom(ctx))
}

// Dump implements Dumper.
func (d *ComposeServiceDumper) Dump(ctx context.Context, engine, containerName string) (io.ReadCloser, error) {
	cmd, err := ComposeDumpCmd(engine)
	if err != nil {
		return nil, err
	}
	rt, err := composeRuntime(ctx, d.Runtime, d.Runtimes)
	if err != nil {
		return nil, fmt.Errorf("backup: dump %s compose service %q: %w", engine, containerName, err)
	}
	rc, err := rt.Exec(ctx, containerName, cmd)
	if err != nil {
		return nil, fmt.Errorf("backup: dump %s compose service %q: %w", engine, containerName, err)
	}
	return rc, nil
}
