package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Errors for the branch table.
var (
	ErrDatabaseBranchNotFound  = errors.New("store: database branch not found")
	ErrDatabaseBranchHasChild  = errors.New("store: database branch has children")
	ErrDatabaseBranchProtected = errors.New("store: database branch is protected")
	ErrDatabaseBranchStateRace = errors.New("store: database branch state changed")
	ErrDatabaseBranchQuota     = errors.New("store: database branch quota reached")
	ErrDatabaseBranchTime      = errors.New("store: database branch time is not RFC3339")
)

// DatabaseBranch is the lineage and lifecycle record of one branch.
type DatabaseBranch struct {
	ID                   string
	DatabaseName         string
	ParentName           string
	RootName             string
	Mode                 string
	Source               string
	SourceRef            string
	ClonePath            string
	ClonePathReason      string
	State                string
	StateReason          string
	Protected            bool
	ExpiresAt            string
	ExpiryPreset         string
	RulesetID            string
	RulesetVersion       int
	PreviewEnvironmentID string
	SizeBytes            int64
	SizeMeasuredAt       string
	LastActiveAt         string
	BranchedAt           string
	CreatedBy            string
	CreatedAt            string
	UpdatedAt            string
}

// BranchQuota bounds a quota checked insert; a zero field means no limit.
type BranchQuota struct {
	MaxPerRoot    int
	MaxTotal      int
	MaxTotalBytes int64
	// ParentBytes is the size the new branch is expected to add.
	ParentBytes int64
}

const databaseBranchColumns = `id, database_name, parent_name, root_name, mode, source, source_ref, clone_path, clone_path_reason, state, state_reason, protected, expires_at, expiry_preset, ruleset_id, ruleset_version, COALESCE(preview_environment_id, ''), size_bytes, size_measured_at, last_active_at, branched_at, created_by, created_at, updated_at`

// branchLiveFilter is the SQL list of states that count against quotas and block a parent delete.
const branchLiveFilter = `state NOT IN ('deleted', 'promoted')`

type branchScanner interface{ Scan(dest ...any) error }

func scanDatabaseBranch(s branchScanner) (DatabaseBranch, error) {
	var b DatabaseBranch
	var prot int
	err := s.Scan(&b.ID, &b.DatabaseName, &b.ParentName, &b.RootName, &b.Mode, &b.Source, &b.SourceRef, &b.ClonePath, &b.ClonePathReason, &b.State, &b.StateReason, &prot, &b.ExpiresAt, &b.ExpiryPreset, &b.RulesetID, &b.RulesetVersion, &b.PreviewEnvironmentID, &b.SizeBytes, &b.SizeMeasuredAt, &b.LastActiveAt, &b.BranchedAt, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	b.Protected = prot == 1
	return b, err
}

// NormalizeBranchTime returns t as UTC RFC3339, so stored times compare as text and as numbers alike. Empty stays empty.
func NormalizeBranchTime(t string) (string, error) {
	if t == "" {
		return "", nil
	}
	p, err := time.Parse(time.RFC3339, t)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrDatabaseBranchTime, t)
	}
	return p.UTC().Format(time.RFC3339), nil
}

func branchInsertArgs(b DatabaseBranch) ([]any, error) {
	prot := 0
	if b.Protected {
		prot = 1
	}
	var preview any
	if b.PreviewEnvironmentID != "" {
		preview = b.PreviewEnvironmentID
	}
	if b.ClonePath == "" {
		b.ClonePath = "dump"
	}
	if b.State == "" {
		b.State = "requested"
	}
	exp, err := NormalizeBranchTime(b.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return []any{b.ID, b.DatabaseName, b.ParentName, b.RootName, b.Mode, b.Source, b.SourceRef, b.ClonePath, b.ClonePathReason, b.State, b.StateReason, prot, exp, b.ExpiryPreset, b.RulesetID, b.RulesetVersion, preview, b.SizeBytes, b.SizeMeasuredAt, b.LastActiveAt, b.BranchedAt, b.CreatedBy, b.CreatedAt, b.UpdatedAt}, nil
}

const branchInsertColumns = `id, database_name, parent_name, root_name, mode, source, source_ref, clone_path, clone_path_reason, state, state_reason, protected, expires_at, expiry_preset, ruleset_id, ruleset_version, preview_environment_id, size_bytes, size_measured_at, last_active_at, branched_at, created_by, created_at, updated_at`

// CreateDatabaseBranch inserts a branch row without a quota check; use CreateDatabaseBranchWithinQuota for a user request.
func (db *DB) CreateDatabaseBranch(ctx context.Context, b DatabaseBranch) error {
	args, err := branchInsertArgs(b)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO database_branches (`+branchInsertColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	if err != nil {
		return fmt.Errorf("store: create database branch %q: %w", b.DatabaseName, err)
	}
	return nil
}

// CreateDatabaseBranchWithinQuota inserts the row only when the live counts allow it, in one statement,
// so two concurrent creates cannot both pass the check. Promoted and deleted rows are not counted.
func (db *DB) CreateDatabaseBranchWithinQuota(ctx context.Context, b DatabaseBranch, q BranchQuota) error {
	args, err := branchInsertArgs(b)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO database_branches (`+branchInsertColumns+`)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE (? <= 0 OR (SELECT COUNT(*) FROM database_branches WHERE root_name = ? AND `+branchLiveFilter+`) < ?)
		  AND (? <= 0 OR (SELECT COUNT(*) FROM database_branches WHERE `+branchLiveFilter+`) < ?)
		  AND (? <= 0 OR (SELECT COALESCE(SUM(size_bytes), 0) FROM database_branches WHERE `+branchLiveFilter+`) + ? <= ?)
	`, reorderQuotaArgs(args, q, b.RootName)...)
	if err != nil {
		return fmt.Errorf("store: create database branch %q: %w", b.DatabaseName, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDatabaseBranchQuota
	}
	return nil
}

// reorderQuotaArgs lays the placeholders out in the order the quota statement reads them.
func reorderQuotaArgs(args []any, q BranchQuota, root string) []any {
	row := args[:24]
	out := append([]any{}, row...)
	return append(out, q.MaxPerRoot, root, q.MaxPerRoot, q.MaxTotal, q.MaxTotal, q.MaxTotalBytes, max(q.ParentBytes, 0), q.MaxTotalBytes)
}

// GetDatabaseBranch returns the branch row of a database name.
func (db *DB) GetDatabaseBranch(ctx context.Context, name string) (DatabaseBranch, error) {
	b, err := scanDatabaseBranch(db.QueryRowContext(ctx, `SELECT `+databaseBranchColumns+` FROM database_branches WHERE database_name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseBranch{}, ErrDatabaseBranchNotFound
	}
	if err != nil {
		return DatabaseBranch{}, fmt.Errorf("store: get database branch %q: %w", name, err)
	}
	return b, nil
}

func (db *DB) listDatabaseBranches(ctx context.Context, where string, args ...any) ([]DatabaseBranch, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+databaseBranchColumns+` FROM database_branches `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list database branches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []DatabaseBranch{}
	for rows.Next() {
		b, err := scanDatabaseBranch(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan database branch: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListDatabaseBranchesByParent returns the direct children of a database.
func (db *DB) ListDatabaseBranchesByParent(ctx context.Context, parent string) ([]DatabaseBranch, error) {
	return db.listDatabaseBranches(ctx, `WHERE parent_name = ? ORDER BY created_at, database_name`, parent)
}

// ListDatabaseBranchesByRoot returns every branch of a tree.
func (db *DB) ListDatabaseBranchesByRoot(ctx context.Context, root string) ([]DatabaseBranch, error) {
	return db.listDatabaseBranches(ctx, `WHERE root_name = ? ORDER BY created_at, database_name`, root)
}

// ListAllDatabaseBranches returns every branch row, for platform quotas.
func (db *DB) ListAllDatabaseBranches(ctx context.Context) ([]DatabaseBranch, error) {
	return db.listDatabaseBranches(ctx, `ORDER BY created_at, database_name`)
}

// UpdateDatabaseBranchState moves a branch from one state to another only when the row id is still in from.
// Moves into resetting, expiring or deleting are refused for a protected branch in the same statement.
func (db *DB) UpdateDatabaseBranchState(ctx context.Context, id, from, to, reason, now string) error {
	guard := ""
	switch to {
	case "resetting", "expiring", "deleting":
		guard = " AND protected = 0"
	}
	set := ""
	if to == "promoted" {
		// The promoted database becomes the root of its own tree, with every descendant.
		set = ", parent_name = '', root_name = database_name, expires_at = '', expiry_preset = ''"
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: update database branch %q state: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE database_branches SET state = ?, state_reason = ?, updated_at = ?`+set+` WHERE id = ? AND state = ?`+guard, to, reason, now, id, from)
	if err != nil {
		return fmt.Errorf("store: update database branch %q state: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Release the single connection before reading the row again.
		_ = tx.Rollback()
		cur, gerr := db.getDatabaseBranchByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if guard != "" && cur.State == from && cur.Protected {
			return ErrDatabaseBranchProtected
		}
		return ErrDatabaseBranchStateRace
	}
	if to == "promoted" {
		if _, err := tx.ExecContext(ctx, `WITH RECURSIVE d(n) AS (
			SELECT database_name FROM database_branches WHERE parent_name = (SELECT database_name FROM database_branches WHERE id = ?)
			UNION
			SELECT b.database_name FROM database_branches b JOIN d ON b.parent_name = d.n
		) UPDATE database_branches SET root_name = (SELECT database_name FROM database_branches WHERE id = ?) WHERE database_name IN (SELECT n FROM d)`, id, id); err != nil {
			return fmt.Errorf("store: re-root branches of %q: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: update database branch %q state: %w", id, err)
	}
	return nil
}

func (db *DB) getDatabaseBranchByID(ctx context.Context, id string) (DatabaseBranch, error) {
	b, err := scanDatabaseBranch(db.QueryRowContext(ctx, `SELECT `+databaseBranchColumns+` FROM database_branches WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseBranch{}, ErrDatabaseBranchNotFound
	}
	if err != nil {
		return DatabaseBranch{}, fmt.Errorf("store: get database branch id %q: %w", id, err)
	}
	return b, nil
}

// SetDatabaseBranchProtected flips protection with a compare and set on the current value,
// and refuses a branch already on its way out.
func (db *DB) SetDatabaseBranchProtected(ctx context.Context, id string, from, to bool, now string) error {
	f, t := 0, 0
	if from {
		f = 1
	}
	if to {
		t = 1
	}
	res, err := db.ExecContext(ctx, `UPDATE database_branches SET protected = ?, updated_at = ? WHERE id = ? AND protected = ? AND state NOT IN ('expiring', 'deleting', 'deleted')`, t, now, id, f)
	if err != nil {
		return fmt.Errorf("store: protect database branch %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := db.getDatabaseBranchByID(ctx, id); gerr != nil {
			return gerr
		}
		return ErrDatabaseBranchStateRace
	}
	return nil
}

// SetDatabaseBranchExpiry changes the expiry (empty for never) of a branch that is not leaving.
func (db *DB) SetDatabaseBranchExpiry(ctx context.Context, id, expiresAt, preset, now string) error {
	exp, err := NormalizeBranchTime(expiresAt)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE database_branches SET expires_at = ?, expiry_preset = ?, updated_at = ? WHERE id = ? AND state NOT IN ('expiring', 'deleting', 'deleted')`, exp, preset, now, id)
	if err != nil {
		return fmt.Errorf("store: set database branch %q expiry: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := db.getDatabaseBranchByID(ctx, id); gerr != nil {
			return gerr
		}
		return ErrDatabaseBranchStateRace
	}
	return nil
}

// DeleteDatabaseBranch removes a branch row in one statement, refusing a protected branch or one with live children.
func (db *DB) DeleteDatabaseBranch(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `
		DELETE FROM database_branches
		WHERE database_name = ? AND protected = 0
		  AND NOT EXISTS (SELECT 1 FROM database_branches c WHERE c.parent_name = database_branches.database_name AND c.`+branchLiveFilter+`)
	`, name)
	if err != nil {
		return fmt.Errorf("store: delete database branch %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	return db.branchDeleteBlocker(ctx, name)
}

// branchDeleteBlocker names why a branch row cannot go, or ErrDatabaseBranchNotFound when there is none.
func (db *DB) branchDeleteBlocker(ctx context.Context, name string) error {
	b, err := db.GetDatabaseBranch(ctx, name)
	if err != nil {
		return err
	}
	if b.Protected {
		return ErrDatabaseBranchProtected
	}
	return ErrDatabaseBranchHasChild
}

// CheckDatabaseDeletable reports ErrDatabaseBranchHasChild or ErrDatabaseBranchProtected when the database may not be deleted.
// Callers run it before touching containers or volumes; the delete itself guards again atomically.
func (db *DB) CheckDatabaseDeletable(ctx context.Context, name string) error {
	var kids, prot int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_branches WHERE parent_name = ? AND `+branchLiveFilter, name).Scan(&kids); err != nil {
		return fmt.Errorf("store: count branch children %q: %w", name, err)
	}
	if kids > 0 {
		return ErrDatabaseBranchHasChild
	}
	err := db.QueryRowContext(ctx, `SELECT protected FROM database_branches WHERE database_name = ?`, name).Scan(&prot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: read branch protection %q: %w", name, err)
	}
	if prot == 1 {
		return ErrDatabaseBranchProtected
	}
	return nil
}

// DuePolicy holds the timings of the one selection rule.
type DuePolicy struct {
	FailedRetention time.Duration
	// Retry is how long an expiring or deleting row may sit before the job resumes it.
	Retry time.Duration
	// RequestedTimeout is how long a requested row may wait for its clone to start.
	RequestedTimeout time.Duration
}

// BranchDue is the one rule for what the expiry job collects. Protected rows never qualify.
// ready: past expires_at. failed: older than FailedRetention. requested: older than RequestedTimeout.
// expiring or deleting: stuck longer than Retry, so the job resumes them. cloning, masking, credentials and
// verifying are not collected here: the boot sweep moves those to failed after the clone timeout, and failed rows then qualify.
// A promoted row has no parent and is never collected.
func BranchDue(b DatabaseBranch, now time.Time, p DuePolicy) bool {
	if b.Protected {
		return false
	}
	age := func(after time.Duration) bool {
		t, err := time.Parse(time.RFC3339, b.UpdatedAt)
		return err == nil && !t.Add(after).After(now)
	}
	switch b.State {
	case "ready":
		t, err := time.Parse(time.RFC3339, b.ExpiresAt)
		return err == nil && !t.After(now)
	case "failed":
		return age(p.FailedRetention)
	case "requested":
		return p.RequestedTimeout > 0 && age(p.RequestedTimeout)
	case "expiring", "deleting":
		return age(p.Retry)
	}
	return false
}

// SelectDueBranches applies BranchDue and drops rows that still have live children.
func SelectDueBranches(rows []DatabaseBranch, now time.Time, p DuePolicy) []DatabaseBranch {
	hasKid := map[string]bool{}
	for _, b := range rows {
		if b.ParentName != "" && b.State != "deleted" && b.State != "promoted" {
			hasKid[b.ParentName] = true
		}
	}
	out := []DatabaseBranch{}
	for _, b := range rows {
		if !hasKid[b.DatabaseName] && BranchDue(b, now, p) {
			out = append(out, b)
		}
	}
	return out
}

// ListExpiredDatabaseBranches returns the branches the expiry job should collect now.
func (db *DB) ListExpiredDatabaseBranches(ctx context.Context, now time.Time, p DuePolicy) ([]DatabaseBranch, error) {
	rows, err := db.ListAllDatabaseBranches(ctx)
	if err != nil {
		return nil, err
	}
	return SelectDueBranches(rows, now, p), nil
}
