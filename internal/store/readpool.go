package store

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// EnvReadConns sizes the read-only connection pool. 0 sends reads to the writer connection.
	EnvReadConns     = "APP_DB_READ_CONNS"
	defaultReadConns = 4
)

func readPoolSize() int {
	if n, err := strconv.Atoi(os.Getenv(EnvReadConns)); err == nil && n >= 0 {
		return n
	}
	return defaultReadConns
}

func isMemoryPath(path string) bool {
	return path == "" || strings.HasPrefix(path, ":") || strings.HasPrefix(path, "file:") ||
		strings.Contains(path, "mode=memory")
}

// openReadPool opens a query_only pool on the same WAL file, or returns nil
// (callers then fall back to the writer) when it cannot or should not exist.
func openReadPool(ctx context.Context, path string) *sql.DB {
	n := readPoolSize()
	if n == 0 || isMemoryPath(path) {
		return nil
	}
	q := url.Values{}
	q.Add("mode", "ro")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "query_only(1)")
	ro, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil
	}
	ro.SetMaxOpenConns(n)
	ro.SetMaxIdleConns(n)
	ro.SetConnMaxIdleTime(5 * time.Minute)
	if err := ro.PingContext(ctx); err != nil {
		_ = ro.Close()
		return nil
	}
	return ro
}

// Close shuts down the read pool and then the writer connection.
func (db *DB) Close() error {
	var roErr error
	if db.ro != nil {
		roErr = db.ro.Close()
	}
	if err := db.DB.Close(); err != nil {
		return err
	}
	return roErr
}

// readConn exposes only query methods, so a call site holding it cannot write.
type readConn struct {
	db *sql.DB
	m  *dbMetrics
}

// reader returns the read pool, or the writer connection when no pool exists.
// Use it only for standalone SELECTs outside transactions and not after a
// write in the same call chain that needs to see uncommitted state.
func (db *DB) reader() readConn {
	if db.ro != nil {
		return readConn{db: db.ro, m: db.m}
	}
	return readConn{db: db.DB, m: db.m}
}

// QueryContext times the statement for the slow query log.
func (r readConn) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := r.db.QueryContext(ctx, query, args...)
	r.m.observe(query, time.Since(start))
	return rows, err
}

// QueryRowContext times the statement for the slow query log.
func (r readConn) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := r.db.QueryRowContext(ctx, query, args...)
	r.m.observe(query, time.Since(start))
	return row
}
