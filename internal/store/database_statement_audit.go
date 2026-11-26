package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrStatementAuditCursor marks a cursor the list could not parse.
var ErrStatementAuditCursor = errors.New("store: invalid statement audit cursor")

// DatabaseStatementAudit is one explorer action. Raw values never live here;
// StatementText is filled only when the live setting allows it.
type DatabaseStatementAudit struct {
	ID           string `json:"id"`
	At           string `json:"at"`
	DatabaseName string `json:"database"`
	// DatabaseID is the instance id of the database when the row was written, so a reused name cannot read an older trail.
	DatabaseID    string `json:"-"`
	Engine        string `json:"engine"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	PrincipalName string `json:"principal_name"`
	Ability       string `json:"ability"`
	Action        string `json:"action"`
	OpKind        string `json:"op_kind"`
	Fingerprint   string `json:"fingerprint"`
	StatementText string `json:"statement_text,omitempty"`
	RowsReturned  int64  `json:"rows"`
	Bytes         int64  `json:"bytes"`
	DurationMs    int64  `json:"duration_ms"`
	Outcome       string `json:"outcome"`
	ErrorClass    string `json:"error_class,omitempty"`
	ClientIP      string `json:"client_ip,omitempty"`
	ClientKind    string `json:"client_kind,omitempty"`
	BranchName    string `json:"branch,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
}

// DatabaseStatementAuditFilter narrows a list. Cursor is the opaque (at, id) of the last row seen.
type DatabaseStatementAuditFilter struct {
	DatabaseName string
	DatabaseID   string
	// MatchDatabaseID makes DatabaseID filter even when empty (a database created before instance ids).
	MatchDatabaseID bool
	PrincipalID     string
	Action          string
	Outcome         string
	Since           string
	Until           string
	Cursor          string
	Limit           int
}

// StatementAuditTimeLayout is fixed width so stored times compare correctly as text.
const StatementAuditTimeLayout = "2006-01-02T15:04:05.000000000Z"

const (
	statementAuditCols     = `id, at, database_name, database_id, engine, principal_type, principal_id, principal_name, ability, action, op_kind, fingerprint, statement_text, rows_returned, bytes, duration_ms, outcome, error_class, client_ip, client_kind, branch_name, request_id`
	statementAuditMaxLimit = 500
	statementAuditDefLimit = 100
)

// InsertDatabaseStatementAudits writes a batch in one transaction.
func (db *DB) InsertDatabaseStatementAudits(ctx context.Context, rows []DatabaseStatementAudit) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin statement audit batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO database_statement_audit (`+statementAuditCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare statement audit: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, r := range rows {
		var branch any
		if r.BranchName != "" {
			branch = r.BranchName
		}
		if _, err := stmt.ExecContext(ctx, r.ID, r.At, r.DatabaseName, r.DatabaseID, r.Engine, r.PrincipalType, r.PrincipalID, r.PrincipalName, r.Ability, r.Action, r.OpKind, r.Fingerprint, r.StatementText, r.RowsReturned, r.Bytes, r.DurationMs, r.Outcome, r.ErrorClass, r.ClientIP, r.ClientKind, branch, r.RequestID); err != nil {
			return fmt.Errorf("store: insert statement audit: %w", err)
		}
	}
	return tx.Commit()
}

// ListDatabaseStatementAudits returns rows newest first. next is the cursor for the following page, empty at the end.
func (db *DB) ListDatabaseStatementAudits(ctx context.Context, f DatabaseStatementAuditFilter) (rows []DatabaseStatementAudit, next string, err error) {
	limit := f.Limit
	if limit <= 0 {
		limit = statementAuditDefLimit
	}
	if limit > statementAuditMaxLimit {
		limit = statementAuditMaxLimit
	}
	where := []string{"database_name = ?"}
	args := []any{f.DatabaseName}
	add := func(cond string, v string) {
		if v != "" {
			where = append(where, cond)
			args = append(args, v)
		}
	}
	if f.MatchDatabaseID {
		where = append(where, "database_id = ?")
		args = append(args, f.DatabaseID)
	}
	add("principal_id = ?", f.PrincipalID)
	add("action = ?", f.Action)
	add("outcome = ?", f.Outcome)
	add("at >= ?", f.Since)
	add("at <= ?", f.Until)
	if f.Cursor != "" {
		at, id, ok := strings.Cut(f.Cursor, "|")
		if !ok {
			return nil, "", ErrStatementAuditCursor
		}
		where = append(where, "(at < ? OR (at = ? AND id < ?))")
		args = append(args, at, at, id)
	}
	args = append(args, limit+1)
	q := `SELECT ` + statementAuditCols + ` FROM database_statement_audit WHERE ` + strings.Join(where, " AND ") + ` ORDER BY at DESC, id DESC LIMIT ?`
	res, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("store: list statement audit: %w", err)
	}
	defer func() { _ = res.Close() }()
	for res.Next() {
		var r DatabaseStatementAudit
		var branch *string
		if err := res.Scan(&r.ID, &r.At, &r.DatabaseName, &r.DatabaseID, &r.Engine, &r.PrincipalType, &r.PrincipalID, &r.PrincipalName, &r.Ability, &r.Action, &r.OpKind, &r.Fingerprint, &r.StatementText, &r.RowsReturned, &r.Bytes, &r.DurationMs, &r.Outcome, &r.ErrorClass, &r.ClientIP, &r.ClientKind, &branch, &r.RequestID); err != nil {
			return nil, "", fmt.Errorf("store: scan statement audit: %w", err)
		}
		if branch != nil {
			r.BranchName = *branch
		}
		rows = append(rows, r)
	}
	if err := res.Err(); err != nil {
		return nil, "", err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = last.At + "|" + last.ID
	}
	return rows, next, nil
}

// PruneDatabaseStatementAudits deletes rows older than cutoff in bounded chunks and returns the count.
func (db *DB) PruneDatabaseStatementAudits(ctx context.Context, cutoff time.Time, chunk int) (int64, error) {
	if chunk <= 0 {
		chunk = statementAuditMaxLimit
	}
	var total int64
	at := cutoff.UTC().Format(StatementAuditTimeLayout)
	for {
		res, err := db.ExecContext(ctx, `DELETE FROM database_statement_audit WHERE id IN (SELECT id FROM database_statement_audit WHERE at < ? LIMIT ?)`, at, chunk)
		if err != nil {
			return total, fmt.Errorf("store: prune statement audit: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n < int64(chunk) {
			return total, nil
		}
	}
}

// TrimDatabaseStatementAudits deletes the oldest rows beyond maxRows, in bounded chunks, and returns the count.
func (db *DB) TrimDatabaseStatementAudits(ctx context.Context, maxRows, chunk int) (int64, error) {
	if maxRows <= 0 {
		return 0, nil
	}
	if chunk <= 0 {
		chunk = statementAuditMaxLimit
	}
	var at, id string
	err := db.QueryRowContext(ctx, `SELECT at, id FROM database_statement_audit ORDER BY at DESC, id DESC LIMIT 1 OFFSET ?`, maxRows-1).Scan(&at, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: locate statement audit cap: %w", err)
	}
	var total int64
	for {
		res, err := db.ExecContext(ctx, `DELETE FROM database_statement_audit WHERE id IN (SELECT id FROM database_statement_audit WHERE at < ? OR (at = ? AND id < ?) LIMIT ?)`, at, at, id, chunk)
		if err != nil {
			return total, fmt.Errorf("store: trim statement audit: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n < int64(chunk) {
			return total, nil
		}
	}
}
