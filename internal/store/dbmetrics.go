package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

const (
	envSlowQueryMS     = "APP_DB_SLOW_QUERY_MS"
	defaultSlowQueryMS = 200
	slowQueryKeep      = 50
	latencyWindow      = 512
	walSizeLimitBytes  = 64 << 20
	maxLoggedStatement = 160
	checkpointPassive  = "PASSIVE"
	checkpointTruncate = "TRUNCATE"
	checkpointEvery    = time.Minute
)

// SlowQuery is one statement that exceeded the slow threshold. Arguments are never recorded.
type SlowQuery struct {
	SQL        string    `json:"sql"`
	DurationMS float64   `json:"duration_ms"`
	At         time.Time `json:"at"`
}

// DBMetrics is a point in time view of database pressure.
type DBMetrics struct {
	Queries         int64       `json:"queries"`
	SlowQueries     int64       `json:"slow_queries"`
	SlowThresholdMS int         `json:"slow_threshold_ms"`
	P50MS           float64     `json:"p50_ms"`
	P95MS           float64     `json:"p95_ms"`
	P99MS           float64     `json:"p99_ms"`
	InUse           int         `json:"in_use"`
	WaitCount       int64       `json:"wait_count"`
	WaitMS          float64     `json:"wait_ms"`
	WALBytes        int64       `json:"wal_bytes"`
	DBBytes         int64       `json:"db_bytes"`
	RecentSlow      []SlowQuery `json:"recent_slow"`
	// Read pool figures. ReadPoolSize is 0 when reads share the writer connection.
	ReadPoolEnabled bool    `json:"read_pool_enabled"`
	ReadPoolSize    int     `json:"read_pool_size"`
	ReadInUse       int     `json:"read_in_use"`
	ReadWaitCount   int64   `json:"read_wait_count"`
	ReadWaitMS      float64 `json:"read_wait_ms"`
}

type dbMetrics struct {
	mu        sync.Mutex
	queries   int64
	slow      int64
	slowAfter time.Duration
	ring      [latencyWindow]time.Duration
	ringN     int
	recent    []SlowQuery
	path      string
}

func newDBMetrics(path string) *dbMetrics {
	ms := defaultSlowQueryMS
	if n, err := strconv.Atoi(os.Getenv(envSlowQueryMS)); err == nil && n > 0 {
		ms = n
	}
	return &dbMetrics{slowAfter: time.Duration(ms) * time.Millisecond, path: path}
}

func (m *dbMetrics) observe(query string, d time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries++
	m.ring[m.ringN%latencyWindow] = d
	m.ringN++
	if d < m.slowAfter {
		return
	}
	m.slow++
	q := strings.Join(strings.Fields(query), " ")
	if len(q) > maxLoggedStatement {
		q = q[:maxLoggedStatement]
	}
	m.recent = append(m.recent, SlowQuery{SQL: q, DurationMS: float64(d) / float64(time.Millisecond), At: time.Now().UTC()})
	if len(m.recent) > slowQueryKeep {
		m.recent = m.recent[len(m.recent)-slowQueryKeep:]
	}
}

// ExecContext times the statement for the slow query log.
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := db.DB.ExecContext(ctx, query, args...)
	db.m.observe(query, time.Since(start))
	return res, err
}

// QueryContext times the statement for the slow query log.
func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := db.DB.QueryContext(ctx, query, args...)
	db.m.observe(query, time.Since(start))
	return rows, err
}

// QueryRowContext times the statement for the slow query log.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := db.DB.QueryRowContext(ctx, query, args...)
	db.m.observe(query, time.Since(start))
	return row
}

// Metrics returns query latency, connection wait and WAL size figures.
func (db *DB) Metrics() DBMetrics {
	out := DBMetrics{}
	st := db.Stats()
	out.InUse, out.WaitCount, out.WaitMS = st.InUse, st.WaitCount, float64(st.WaitDuration)/float64(time.Millisecond)
	if db.ro != nil {
		rs := db.ro.Stats()
		out.ReadPoolEnabled, out.ReadPoolSize = true, rs.MaxOpenConnections
		out.ReadInUse, out.ReadWaitCount = rs.InUse, rs.WaitCount
		out.ReadWaitMS = float64(rs.WaitDuration) / float64(time.Millisecond)
	}
	m := db.m
	if m == nil {
		return out
	}
	m.mu.Lock()
	n := min(m.ringN, latencyWindow)
	win := make([]time.Duration, n)
	copy(win, m.ring[:n])
	out.Queries, out.SlowQueries = m.queries, m.slow
	out.SlowThresholdMS = int(m.slowAfter / time.Millisecond)
	out.RecentSlow = append([]SlowQuery(nil), m.recent...)
	path := m.path
	m.mu.Unlock()
	sort.Slice(win, func(i, j int) bool { return win[i] < win[j] })
	pick := func(p float64) float64 {
		if n == 0 {
			return 0
		}
		return float64(win[min(n-1, int(p*float64(n)))]) / float64(time.Millisecond)
	}
	out.P50MS, out.P95MS, out.P99MS = pick(0.50), pick(0.95), pick(0.99)
	if path != "" && !strings.HasPrefix(path, ":") && !strings.HasPrefix(path, "file:") {
		if fi, err := os.Stat(path + "-wal"); err == nil {
			out.WALBytes = fi.Size()
		}
		if fi, err := os.Stat(path); err == nil {
			out.DBBytes = fi.Size()
		}
	}
	return out
}

// ResetSlowQueries clears the slow query counter and list.
func (db *DB) ResetSlowQueries() {
	if db.m == nil {
		return
	}
	db.m.mu.Lock()
	db.m.slow, db.m.recent = 0, nil
	db.m.mu.Unlock()
}

// CheckpointWAL folds the write ahead log into the database file. truncate also resets the file to zero bytes.
func (db *DB) CheckpointWAL(ctx context.Context, truncate bool) error {
	mode := checkpointPassive
	if truncate {
		mode = checkpointTruncate
	}
	var busy, logFrames, done int
	if err := db.DB.QueryRowContext(ctx, "PRAGMA wal_checkpoint("+mode+")").Scan(&busy, &logFrames, &done); err != nil {
		return fmt.Errorf("store: wal checkpoint %s: %w", mode, err)
	}
	return nil
}

// Optimize refreshes planner statistics for tables whose shape changed.
func (db *DB) Optimize(ctx context.Context) error {
	if _, err := db.DB.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return fmt.Errorf("store: optimize: %w", err)
	}
	return nil
}

// RunWALMaintenance passively checkpoints on an interval so a long reader never lets the log grow unbounded.
func (db *DB) RunWALMaintenance(ctx context.Context) {
	t := time.NewTicker(checkpointEvery)
	defer t.Stop()
	loop := footprint.Track("wal checkpoint", checkpointEvery)
	defer loop.Active()()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			start := time.Now()
			err := db.CheckpointWAL(ctx, false)
			loop.Observe(start, err)
		}
	}
}
