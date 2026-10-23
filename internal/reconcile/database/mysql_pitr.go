package database

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// MySQLBinlogPath is where a PITR-enabled MySQL or MariaDB database mounts its
// binlog volume. The backup package reads the same path to ship closed files.
const MySQLBinlogPath = "/var/lib/mysql-binlog"

// MySQLBinlogBasename is the log-bin prefix inside MySQLBinlogPath.
const MySQLBinlogBasename = "mysql-bin"

// EnvBinlogExpireSeconds bounds how long a database keeps closed binlog files
// locally; shipped copies in the backup target are pruned by base backup retention.
const EnvBinlogExpireSeconds = "APP_PITR_BINLOG_EXPIRE_SECONDS"

const defaultBinlogExpireSeconds = 7 * 24 * 3600

func isMySQLFamily(engine string) bool {
	return engine == store.EngineMySQL || engine == store.EngineMariaDB
}

// SupportsPITR reports whether engine can run with point-in-time restore enabled.
func SupportsPITR(engine string) bool {
	return engine == store.EnginePostgres || isMySQLFamily(engine)
}

func binlogExpireSeconds() int {
	if v := os.Getenv(EnvBinlogExpireSeconds); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultBinlogExpireSeconds
}

// ROW format keeps replay deterministic and lets the restore rewrite the schema name.
func mysqlBinlogFlags() []string {
	return []string{
		"--log-bin=" + MySQLBinlogPath + "/" + MySQLBinlogBasename,
		"--server-id=1",
		"--binlog-format=ROW",
		"--binlog-expire-logs-seconds=" + strconv.Itoa(binlogExpireSeconds()),
	}
}

func (c *Controller) provisionBinlogVolume(ctx context.Context, volName string) error {
	id, err := createCertsHelper(ctx, c.runtime, volName)
	if err != nil {
		return fmt.Errorf("provision binlog volume: %w", err)
	}
	defer func() { _ = c.runtime.Remove(context.Background(), id, true) }()
	cmd := []string{"chown", "-R", fmt.Sprintf("%d:%d", dbTLSUID, dbTLSGID), certsHelperMountPath}
	rc, err := c.runtime.Exec(ctx, id, cmd)
	if err != nil {
		return fmt.Errorf("provision binlog volume: chown: %w", err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("provision binlog volume: chown: %w", err)
	}
	return nil
}
