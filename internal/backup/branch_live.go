package backup

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/store"
)

// Consistency labels a branch dump records, so a best effort copy is never presented as a snapshot.
const (
	ConsistencySnapshot   = "snapshot"
	ConsistencyBestEffort = "best_effort"
)

// BranchDumper streams a consistent live dump of a parent database for the branch clone worker.
type BranchDumper struct {
	Runtime docker.Runtime
}

// mongoReplicaProbe prints the replica set name, empty on a standalone server.
var mongoReplicaProbe = []string{"sh", "-c", `exec mongosh --quiet --host 127.0.0.1 --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'print(db.hello().setName || "")'`}

// Dump returns the dump stream and its consistency label. schemaOnly is supported for the SQL engines only.
func (d *BranchDumper) Dump(ctx context.Context, engine, containerName string, schemaOnly bool) (io.ReadCloser, string, error) {
	cmd, consistency, err := d.command(ctx, engine, containerName, schemaOnly)
	if err != nil {
		return nil, "", err
	}
	rc, err := d.Runtime.Exec(ctx, containerName, cmd)
	if err != nil {
		return nil, "", fmt.Errorf("backup: branch dump %s container %q: %w", engine, containerName, err)
	}
	return rc, consistency, nil
}

func (d *BranchDumper) command(ctx context.Context, engine, containerName string, schemaOnly bool) ([]string, string, error) {
	schema := func(flag string) string {
		if schemaOnly {
			return " " + flag
		}
		return ""
	}
	switch engine {
	case store.EnginePostgres:
		// pg_dump reads one repeatable read snapshot by itself.
		return []string{"sh", "-c", `exec pg_dump --no-password --no-owner --no-privileges` + schema("--schema-only") + ` -U "$POSTGRES_USER" "$POSTGRES_USER"`}, ConsistencySnapshot, nil
	case store.EngineMySQL:
		return []string{"sh", "-c", `exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines --triggers --events` + schema("--no-data") + ` "$MYSQL_DATABASE"`}, ConsistencySnapshot, nil
	case store.EngineMariaDB:
		return []string{"sh", "-c", `exec mariadb-dump -uroot -p"$MARIADB_ROOT_PASSWORD" --single-transaction --routines --triggers --events` + schema("--no-data") + ` "$MARIADB_DATABASE"`}, ConsistencySnapshot, nil
	}
	if schemaOnly {
		return nil, "", fmt.Errorf("backup: branch dump: schema only is not available for %s", engine)
	}
	switch engine {
	case store.EngineMongoDB:
		if d.isReplicaSet(ctx, containerName) {
			// The restore does not replay the oplog, so the copy is only as consistent as a plain dump.
			return []string{"sh", "-c", `exec mongodump --archive --oplog --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`}, ConsistencyBestEffort, nil
		}
		return mongoDumpCmd, ConsistencyBestEffort, nil
	case store.EngineRedis, store.EngineValkey:
		return redisDumpCmd, ConsistencySnapshot, nil
	case store.EngineKeyDB:
		return keydbDumpCmd, ConsistencySnapshot, nil
	case store.EngineDragonfly:
		return dragonflyDumpCmd, ConsistencySnapshot, nil
	}
	return nil, "", fmt.Errorf("backup: branch dump: engine %q is not supported", engine)
}

// isReplicaSet is false on any probe failure, which only costs the oplog flag and is labeled best effort.
func (d *BranchDumper) isReplicaSet(ctx context.Context, containerName string) bool {
	rc, err := d.Runtime.Exec(ctx, containerName, mongoReplicaProbe)
	if err != nil {
		return false
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, 256))
	return err == nil && strings.TrimSpace(string(b)) != ""
}

// sizeCmds print the data directory size in KiB.
var sizeCmds = map[string]string{
	store.EnginePostgres: `du -sk "${PGDATA:-/var/lib/postgresql/data}"`,
	store.EngineMySQL:    `du -sk /var/lib/mysql`,
	store.EngineMariaDB:  `du -sk /var/lib/mysql`,
	store.EngineMongoDB:  `du -sk /data/db`,
	store.EngineRedis:    `du -sk /data`, store.EngineValkey: `du -sk /data`, store.EngineKeyDB: `du -sk /data`, store.EngineDragonfly: `du -sk /data`,
}

// countCmds print one number: tables for SQL engines, collections for MongoDB, keys for the Redis family.
var countCmds = map[string]string{
	store.EnginePostgres:  `exec psql -tA --no-password -U "$POSTGRES_USER" -c "select count(*) from information_schema.tables where table_schema not in ('pg_catalog','information_schema')"`,
	store.EngineMySQL:     `exec mysql -N -uroot -p"$MYSQL_ROOT_PASSWORD" -e "select count(*) from information_schema.tables where table_schema='$MYSQL_DATABASE'"`,
	store.EngineMariaDB:   `exec mariadb -N -uroot -p"$MARIADB_ROOT_PASSWORD" -e "select count(*) from information_schema.tables where table_schema='$MARIADB_DATABASE'"`,
	store.EngineMongoDB:   `exec mongosh --quiet --host 127.0.0.1 --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'print(db.adminCommand({listDatabases:1}).databases.filter(d=>!["admin","local","config"].includes(d.name)).reduce((a,d)=>a+db.getSiblingDB(d.name).getCollectionNames().length,0))'`,
	store.EngineRedis:     redisTLSProbe + ` exec redis-cli $RTLS dbsize`,
	store.EngineValkey:    redisTLSProbe + ` exec redis-cli $RTLS dbsize`,
	store.EngineKeyDB:     redisTLSProbe + ` exec keydb-cli $RTLS dbsize`,
	store.EngineDragonfly: `exec redis-cli dbsize`,
}

func (d *BranchDumper) execNumber(ctx context.Context, containerName, script string) (int64, error) {
	rc, err := d.Runtime.Exec(ctx, containerName, []string{"sh", "-c", script})
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, 4096))
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	for i := len(fields) - 1; i >= 0; i-- {
		var n int64
		if _, err := fmt.Sscanf(fields[i], "%d", &n); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("backup: branch measure: no number in output")
}

// Size returns the data directory size of a database container in bytes.
func (d *BranchDumper) Size(ctx context.Context, engine, containerName string) (int64, error) {
	script, ok := sizeCmds[engine]
	if !ok {
		return 0, fmt.Errorf("backup: branch size: engine %q is not supported", engine)
	}
	kib, err := d.execNumber(ctx, containerName, script)
	return kib * 1024, err
}

// Count returns the table, collection or key count used to compare a branch with its parent.
func (d *BranchDumper) Count(ctx context.Context, engine, containerName string) (int64, error) {
	script, ok := countCmds[engine]
	if !ok {
		return 0, fmt.Errorf("backup: branch count: engine %q is not supported", engine)
	}
	return d.execNumber(ctx, containerName, script)
}
