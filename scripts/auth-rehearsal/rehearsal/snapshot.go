package rehearsal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Snapshot is a content digest per table plus the row set of the tables a
// backfill is allowed to add to.
type Snapshot struct {
	Digest map[string]string
	Rows   map[string]int
	sets   map[string]map[string]struct{}
}

// secretTables may gain rows (the engine encryption key) but never change existing ones.
var secretTables = map[string]bool{"service_secrets": true, "service_secret_values": true}

func isEngineTable(n string) bool {
	return strings.HasPrefix(n, "theauth_") || strings.HasPrefix(n, "authengine_")
}

// TakeSnapshot digests every legacy table (engine=false) or every library and
// mapping table (engine=true) of db.
func TakeSnapshot(ctx context.Context, db *sql.DB, engine bool) (*Snapshot, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("rehearsal: list tables: %w", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("rehearsal: scan table name: %w", err)
		}
		if isEngineTable(n) == engine {
			names = append(names, n)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("rehearsal: close table list: %w", err)
	}
	s := &Snapshot{Digest: map[string]string{}, Rows: map[string]int{}, sets: map[string]map[string]struct{}{}}
	for _, n := range names {
		lines, err := tableLines(ctx, db, n)
		if err != nil {
			return nil, err
		}
		sort.Strings(lines)
		h := sha256.New()
		for _, l := range lines {
			_, _ = h.Write([]byte(l))
			_, _ = h.Write([]byte{'\n'})
		}
		s.Digest[n], s.Rows[n] = hex.EncodeToString(h.Sum(nil)), len(lines)
		if secretTables[n] {
			set := make(map[string]struct{}, len(lines))
			for _, l := range lines {
				set[l] = struct{}{}
			}
			s.sets[n] = set
		}
	}
	return s, nil
}

func tableLines(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT * FROM `+table) //nolint:gosec // table name comes from sqlite_master
	if err != nil {
		return nil, fmt.Errorf("rehearsal: read %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("rehearsal: columns of %s: %w", table, err)
	}
	var out []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("rehearsal: scan %s: %w", table, err)
		}
		var b strings.Builder
		for _, v := range vals {
			switch x := v.(type) {
			case []byte:
				fmt.Fprintf(&b, "b:%x|", x)
			default:
				fmt.Fprintf(&b, "%T:%v|", x, x)
			}
		}
		out = append(out, b.String())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rehearsal: iterate %s: %w", table, err)
	}
	return out, nil
}

// DiffLegacy lists tables whose content differs between two legacy snapshots.
// Secret tables may only gain rows; every other table must be identical.
func DiffLegacy(before, after *Snapshot) []string {
	var diffs []string
	for n, d := range before.Digest {
		if after.Digest[n] == d {
			continue
		}
		if !secretTables[n] {
			diffs = append(diffs, fmt.Sprintf("%s changed (%d -> %d rows)", n, before.Rows[n], after.Rows[n]))
			continue
		}
		for l := range before.sets[n] {
			if _, ok := after.sets[n][l]; !ok {
				diffs = append(diffs, n+": an existing row was modified or removed")
				break
			}
		}
	}
	sort.Strings(diffs)
	return diffs
}

// DiffExact lists tables that differ at all between two snapshots.
func DiffExact(before, after *Snapshot) []string {
	var diffs []string
	for n, d := range after.Digest {
		if before.Digest[n] != d {
			diffs = append(diffs, fmt.Sprintf("%s changed (%d -> %d rows)", n, before.Rows[n], after.Rows[n]))
		}
	}
	sort.Strings(diffs)
	return diffs
}

// EngineRowTotal is the number of rows across all library and mapping tables.
func (s *Snapshot) EngineRowTotal() int {
	n := 0
	for _, r := range s.Rows {
		n += r
	}
	return n
}

// ObservedCounts polls the library users table from its own connection until
// stop is closed and returns every distinct count seen. More than the values
// 0 and N means the backfill committed in more than one transaction.
func ObservedCounts(ctx context.Context, dbPath string, stop <-chan struct{}) (map[int]bool, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("rehearsal: open poller: %w", err)
	}
	defer func() { _ = db.Close() }()
	seen := map[int]bool{}
	for {
		select {
		case <-stop:
			return seen, nil
		case <-ctx.Done():
			return seen, nil
		default:
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM theauth_users`).Scan(&n); err == nil {
			seen[n] = true
		}
		time.Sleep(2 * time.Millisecond)
	}
}
