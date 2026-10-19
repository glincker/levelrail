package archive

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const maxBundlesPerRun = 20

var restoreTables = map[string]bool{
	"audit_log": true, "deploy_attempts": true,
	"pipeline_runs": true, "pipeline_jobs": true, "pipeline_steps": true,
}

// Manager creates, lists, restores and prunes bundles in one directory.
type Manager struct {
	db  *sql.DB
	dir string
	now func() time.Time
	mu  sync.Mutex
}

// New returns a Manager storing bundles under dataDir/archives.
func New(db *sql.DB, dataDir string) *Manager {
	return &Manager{db: db, dir: filepath.Join(dataDir, DirName), now: time.Now}
}

// Create archives every cold row of kind older than cutoff into one bundle and
// removes those rows from the live tables once the bundle has been re-read and
// matches. It returns ErrNothingToArchive when no row qualifies.
func (m *Manager) Create(ctx context.Context, kind Kind, cutoff time.Time) (Bundle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.create(ctx, kind, cutoff)
}

func (m *Manager) create(ctx context.Context, kind Kind, cutoff time.Time) (Bundle, error) {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return Bundle{}, fmt.Errorf("archive: create dir: %w", err)
	}
	now := m.now().UTC()
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return Bundle{}, fmt.Errorf("archive: id: %w", err)
	}
	id := fmt.Sprintf("%s-%s-%s", kind, now.Format(nameLayout), hex.EncodeToString(suffix))
	w, err := newBundleWriter(m.bundlePath(id))
	if err != nil {
		return Bundle{}, err
	}
	cleanup := func() { _ = os.Remove(m.bundlePath(id)) }
	targets, err := collect(ctx, m.db, kind, cutoff, w)
	if err != nil {
		_, _, _ = w.finish()
		cleanup()
		return Bundle{}, err
	}
	if w.total == 0 {
		_, _, _ = w.finish()
		cleanup()
		return Bundle{}, ErrNothingToArchive
	}
	size, sum, err := w.finish()
	if err != nil {
		cleanup()
		return Bundle{}, err
	}
	b := Bundle{ID: id, Kind: kind, CreatedAt: now, Cutoff: cutoff.UTC(), Rows: w.rows, TotalRows: w.total, SizeBytes: size, SHA256: sum}
	if err := m.checkContents(b); err != nil {
		cleanup()
		return Bundle{}, fmt.Errorf("archive: bundle failed re-read, live rows kept: %w", err)
	}
	if err := m.writeMeta(b); err != nil {
		cleanup()
		return Bundle{}, err
	}
	if err := deleteTargets(ctx, m.db, targets); err != nil {
		return Bundle{}, fmt.Errorf("archive: bundle %s written but live rows not removed: %w", id, err)
	}
	return b, nil
}

// checkContents re-reads the bundle and compares hash and row counts to b.
func (m *Manager) checkContents(b Bundle) error {
	sum, err := fileSHA256(m.bundlePath(b.ID))
	if err != nil {
		return err
	}
	if sum != b.SHA256 {
		return ErrChecksum
	}
	counts := map[string]int{}
	if err := m.scanBundle(b.ID, func(l line) (bool, error) {
		counts[l.Table]++
		return true, nil
	}); err != nil {
		return err
	}
	for t, n := range b.Rows {
		if counts[t] != n {
			return fmt.Errorf("archive: %s has %d rows, expected %d", t, counts[t], n)
		}
	}
	if len(counts) != len(b.Rows) {
		return fmt.Errorf("archive: unexpected tables in bundle")
	}
	return nil
}

// List returns every bundle, newest first.
func (m *Manager) List() ([]Bundle, error) {
	entries, err := os.ReadDir(m.dir)
	if os.IsNotExist(err) {
		return []Bundle{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("archive: list: %w", err)
	}
	out := []Bundle{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, metaSuffix) {
			continue
		}
		b, err := m.readMeta(strings.TrimSuffix(name, metaSuffix))
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Get returns one bundle.
func (m *Manager) Get(id string) (Bundle, error) { return m.readMeta(id) }

// Path returns the bundle file path for download after confirming it exists.
func (m *Manager) Path(id string) (string, error) {
	if _, err := m.readMeta(id); err != nil {
		return "", err
	}
	return m.bundlePath(id), nil
}

// Delete removes a bundle and its metadata.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.readMeta(id); err != nil {
		return err
	}
	if err := os.Remove(m.bundlePath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("archive: delete bundle: %w", err)
	}
	if err := os.Remove(m.metaPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("archive: delete meta: %w", err)
	}
	return nil
}

// Rows is one page of a bundle's rows for one table.
type Rows struct {
	Table string           `json:"table"`
	Total int              `json:"total"`
	Rows  []map[string]any `json:"rows"`
}

// Browse returns up to limit rows of table starting at offset, without
// touching the live database.
func (m *Manager) Browse(id, table string, offset, limit int) (Rows, error) {
	b, err := m.readMeta(id)
	if err != nil {
		return Rows{}, err
	}
	if _, ok := b.Rows[table]; !ok {
		return Rows{}, fmt.Errorf("archive: %s has no %s rows: %w", id, table, ErrNotFound)
	}
	limit = max(1, min(limit, 200))
	res := Rows{Table: table, Total: b.Rows[table], Rows: []map[string]any{}}
	seen := 0
	err = m.scanBundle(id, func(l line) (bool, error) {
		if l.Table != table {
			return true, nil
		}
		if seen >= offset {
			row, derr := decodeRowAny(l.Row)
			if derr != nil {
				return false, derr
			}
			res.Rows = append(res.Rows, row)
		}
		seen++
		return len(res.Rows) < limit, nil
	})
	return res, err
}

func decodeRowAny(raw map[string]json.RawMessage) (map[string]any, error) {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		val, err := decodeValue(v)
		if err != nil {
			return nil, fmt.Errorf("archive: decode %s: %w", k, err)
		}
		out[k] = val
	}
	return out, nil
}

func decodeValue(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		f, err := n.Float64()
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	return v, nil
}

// RestoreResult reports how many rows came back per table.
type RestoreResult struct {
	Restored map[string]int `json:"restored"`
	Existing int            `json:"already_present"`
	Skipped  int            `json:"skipped_missing_parent"`
}

// Restore puts a bundle's rows back into the live tables. Rows already
// present are left alone; pipeline runs whose pipeline was deleted are skipped.
func (m *Manager) Restore(ctx context.Context, id string) (RestoreResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, err := m.readMeta(id)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := m.checkContents(b); err != nil {
		return RestoreResult{}, err
	}
	res, err := m.restoreInto(ctx, m.db, id, true)
	if err != nil {
		return RestoreResult{}, err
	}
	now := m.now().UTC()
	b.RestoredAt = &now
	if err := m.writeMeta(b); err != nil {
		return res, err
	}
	return res, nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT name FROM pragma_table_info('"+table+"')") //nolint:gosec // table is checked against restoreTables
	if err != nil {
		return nil, fmt.Errorf("archive: table info %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	cols := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("archive: scan table info: %w", err)
		}
		cols[n] = true
	}
	return cols, rows.Err()
}

func (m *Manager) restoreInto(ctx context.Context, db *sql.DB, id string, checkParents bool) (RestoreResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("archive: begin restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res := RestoreResult{Restored: map[string]int{}}
	known := map[string]map[string]bool{}
	skipRuns, skipJobs := map[string]bool{}, map[string]bool{}
	err = m.scanBundle(id, func(l line) (bool, error) {
		if !restoreTables[l.Table] {
			return false, fmt.Errorf("archive: bundle holds unknown table %q", l.Table)
		}
		row, derr := decodeRowAny(l.Row)
		if derr != nil {
			return false, derr
		}
		if checkParents {
			skip, perr := parentMissing(ctx, tx, l.Table, row, skipRuns, skipJobs)
			if perr != nil {
				return false, perr
			}
			if skip {
				res.Skipped++
				return true, nil
			}
		}
		cols, ok := known[l.Table]
		if !ok {
			var cerr error
			if cols, cerr = tableColumns(ctx, tx, l.Table); cerr != nil {
				return false, cerr
			}
			known[l.Table] = cols
		}
		names := make([]string, 0, len(row))
		for c := range row {
			if cols[c] {
				names = append(names, c)
			}
		}
		sort.Strings(names)
		args := make([]any, len(names))
		for i, c := range names {
			args[i] = row[c]
		}
		q := "INSERT OR IGNORE INTO " + l.Table + " (" + strings.Join(names, ",") + ") VALUES (?" + strings.Repeat(",?", len(names)-1) + ")" //nolint:gosec // table and columns come from fixed sets, values are bound
		r, eerr := tx.ExecContext(ctx, q, args...)
		if eerr != nil {
			return false, fmt.Errorf("archive: restore %s row: %w", l.Table, eerr)
		}
		if n, _ := r.RowsAffected(); n > 0 {
			res.Restored[l.Table]++
		} else {
			res.Existing++
		}
		return true, nil
	})
	if err != nil {
		return RestoreResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RestoreResult{}, fmt.Errorf("archive: commit restore: %w", err)
	}
	return res, nil
}

func parentMissing(ctx context.Context, tx *sql.Tx, table string, row map[string]any, skipRuns, skipJobs map[string]bool) (bool, error) {
	switch table {
	case "pipeline_runs":
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM pipelines WHERE id = ?`, row["pipeline_id"]).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			skipRuns[fmt.Sprint(row["id"])] = true
			return true, nil
		}
		return false, err
	case "pipeline_jobs":
		if skipRuns[fmt.Sprint(row["run_id"])] {
			skipJobs[fmt.Sprint(row["id"])] = true
			return true, nil
		}
	case "pipeline_steps":
		return skipJobs[fmt.Sprint(row["job_id"])], nil
	}
	return false, nil
}

// Verify checks the bundle's hash and row counts, then restores it into a
// scratch database built from the current schema and compares row counts.
func (m *Manager) Verify(ctx context.Context, id string) (Bundle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, err := m.readMeta(id)
	if err != nil {
		return Bundle{}, err
	}
	verr := m.verifyByRestore(ctx, b)
	now := m.now().UTC()
	ok := verr == nil
	b.VerifiedAt, b.VerifiedOK = &now, &ok
	if err := m.writeMeta(b); err != nil {
		return b, err
	}
	return b, verr
}

func (m *Manager) verifyByRestore(ctx context.Context, b Bundle) error {
	if err := m.checkContents(b); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "archive-verify-")
	if err != nil {
		return fmt.Errorf("archive: scratch dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	scratch, err := store.Open(ctx, filepath.Join(tmp, "scratch.db"))
	if err != nil {
		return fmt.Errorf("archive: open scratch db: %w", err)
	}
	defer func() { _ = scratch.Close() }()
	// Parents are absent in the scratch db, so enforcement is off for it only.
	if _, err := scratch.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("archive: scratch pragma: %w", err)
	}
	res, err := m.restoreInto(ctx, scratch.DB, b.ID, false)
	if err != nil {
		return fmt.Errorf("archive: scratch restore: %w", err)
	}
	for t, n := range b.Rows {
		if res.Restored[t] != n {
			return fmt.Errorf("archive: scratch restore of %s gave %d rows, expected %d", t, res.Restored[t], n)
		}
	}
	return nil
}

// Prune deletes bundles older than keepDays (0 keeps everything).
func (m *Manager) Prune(keepDays int) (int, error) {
	if keepDays <= 0 {
		return 0, nil
	}
	list, err := m.List()
	if err != nil {
		return 0, err
	}
	cutoff := m.now().UTC().AddDate(0, 0, -keepDays)
	n := 0
	for _, b := range list {
		if b.CreatedAt.Before(cutoff) {
			if err := m.Delete(b.ID); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// RunResult summarises one archive pass.
type RunResult struct {
	Created []Bundle `json:"created"`
	Pruned  int      `json:"pruned"`
}

// Run archives every kind per the policy, then prunes expired bundles.
func (m *Manager) Run(ctx context.Context, p store.ArchiveSettings) (RunResult, error) {
	hot := map[Kind]int{KindAudit: p.AuditHotDays, KindDeploys: p.DeployHotDays, KindPipelines: p.PipelineHotDays}
	res := RunResult{Created: []Bundle{}}
	now := m.now().UTC()
	for _, k := range Kinds {
		for range maxBundlesPerRun {
			b, err := m.Create(ctx, k, now.AddDate(0, 0, -max(hot[k], 1)))
			if errors.Is(err, ErrNothingToArchive) {
				break
			}
			if err != nil {
				return res, fmt.Errorf("archive %s: %w", k, err)
			}
			res.Created = append(res.Created, b)
		}
	}
	n, err := m.Prune(p.KeepDays)
	res.Pruned = n
	if err != nil {
		return res, fmt.Errorf("prune archives: %w", err)
	}
	return res, nil
}

// HotRows counts the live rows per kind, to show what is still hot.
func (m *Manager) HotRows(ctx context.Context) map[string]int {
	out := map[string]int{}
	for k, table := range map[Kind]string{KindAudit: "audit_log", KindDeploys: "deploy_attempts", KindPipelines: "pipeline_runs"} {
		var n int
		if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err == nil { //nolint:gosec // fixed table names
			out[string(k)] = n
		}
	}
	return out
}
