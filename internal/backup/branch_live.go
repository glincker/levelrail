package backup

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
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
			return []string{"sh", "-c", `exec mongodump --archive --oplog --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin`}, ConsistencySnapshot, nil
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
