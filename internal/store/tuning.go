package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"
)

// Tuning is the SQLite connection tuning; zero values and "" keep SQLite defaults.
type Tuning struct {
	CacheSizeMB int
	MmapSizeMB  int
	TempStore   string // "file" or "memory"
	Synchronous string // "normal" or "full"
}

var tuningSource atomic.Pointer[func() Tuning]

// SetTuningSource installs the runtime setting reader polled by the WAL tick.
func SetTuningSource(fn func() Tuning) { tuningSource.Store(&fn) }

// pragmas builds the statements; the values are enum or bounded ints, never caller text.
func (t Tuning) pragmas() []string {
	var out []string
	if t.CacheSizeMB > 0 {
		out = append(out, fmt.Sprintf("PRAGMA cache_size = -%d", t.CacheSizeMB*1024))
	}
	if t.MmapSizeMB > 0 {
		out = append(out, fmt.Sprintf("PRAGMA mmap_size = %d", t.MmapSizeMB*1024*1024))
	}
	switch t.TempStore {
	case "file":
		out = append(out, "PRAGMA temp_store = FILE")
	case "memory":
		out = append(out, "PRAGMA temp_store = MEMORY")
	}
	switch t.Synchronous {
	case "normal":
		out = append(out, "PRAGMA synchronous = NORMAL")
	case "full":
		out = append(out, "PRAGMA synchronous = FULL")
	}
	return out
}

// ApplyTuning runs the pragmas on the writer connection and on every read connection.
// Reconnected read conns pick the values up on the next tick.
func (db *DB) ApplyTuning(ctx context.Context, t Tuning) error {
	stmts := t.pragmas()
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("store: tuning %q: %w", s, err)
		}
	}
	if db.ro == nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	n := db.ro.Stats().MaxOpenConnections
	conns := make([]*sql.Conn, 0, n)
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	// Holding all conns at once forces the pool to hand out each one exactly once.
	for i := 0; i < n; i++ {
		c, err := db.ro.Conn(cctx)
		if err != nil {
			break
		}
		conns = append(conns, c)
		for _, s := range stmts {
			if _, err := c.ExecContext(cctx, s); err != nil {
				return fmt.Errorf("store: tuning read conn %q: %w", s, err)
			}
		}
	}
	return nil
}

// ApplyTuningSource applies the installed source, if any.
func (db *DB) ApplyTuningSource(ctx context.Context) {
	if fn := tuningSource.Load(); fn != nil {
		_ = db.ApplyTuning(ctx, (*fn)())
	}
}
