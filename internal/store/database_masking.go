package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Masking rules belong to the database name and go with it, including on a soft delete and undo,
// the same way a recreated name starts with no branch row.

// MaskingRuleset is one saved version of the masking rules of a database.
type MaskingRuleset struct {
	ID           string
	DatabaseName string
	Version      int
	CreatedBy    string
	CreatedAt    string
}

// MaskingRule is one column or field rule; Params never holds a secret value.
type MaskingRule struct {
	ID            string
	RulesetID     string
	Target        string
	Kind          string
	Params        string
	Deterministic bool
	SaltRef       string
	Position      int
}

// SaveMaskingRuleset stores the next version of a database's rules with its rules in one transaction.
func (db *DB) SaveMaskingRuleset(ctx context.Context, rs MaskingRuleset, rules []MaskingRule) (MaskingRuleset, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return MaskingRuleset{}, fmt.Errorf("store: begin masking ruleset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM database_masking_rulesets WHERE database_name = ?`, rs.DatabaseName).Scan(&rs.Version); err != nil {
		return MaskingRuleset{}, fmt.Errorf("store: next masking version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_masking_rulesets (id, database_name, version, created_by, created_at) VALUES (?, ?, ?, ?, ?)`, rs.ID, rs.DatabaseName, rs.Version, rs.CreatedBy, rs.CreatedAt); err != nil {
		return MaskingRuleset{}, fmt.Errorf("store: insert masking ruleset: %w", err)
	}
	for i, r := range rules {
		det := 0
		if r.Deterministic {
			det = 1
		}
		params := r.Params
		if params == "" {
			params = "{}"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO database_masking_rules (id, ruleset_id, target, kind, params, deterministic, salt_ref, position) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, rs.ID, r.Target, r.Kind, params, det, r.SaltRef, i); err != nil {
			return MaskingRuleset{}, fmt.Errorf("store: insert masking rule %q: %w", r.Target, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return MaskingRuleset{}, fmt.Errorf("store: commit masking ruleset: %w", err)
	}
	return rs, nil
}

// GetLatestMaskingRuleset returns the newest version for a database and whether one exists.
func (db *DB) GetLatestMaskingRuleset(ctx context.Context, name string) (MaskingRuleset, []MaskingRule, bool, error) {
	var rs MaskingRuleset
	rows, err := db.QueryContext(ctx, `SELECT id, database_name, version, created_by, created_at FROM database_masking_rulesets WHERE database_name = ? ORDER BY version DESC LIMIT 1`, name)
	if err != nil {
		return rs, nil, false, fmt.Errorf("store: latest masking ruleset: %w", err)
	}
	found := false
	if rows.Next() {
		found = true
		if err := rows.Scan(&rs.ID, &rs.DatabaseName, &rs.Version, &rs.CreatedBy, &rs.CreatedAt); err != nil {
			_ = rows.Close()
			return rs, nil, false, fmt.Errorf("store: scan masking ruleset: %w", err)
		}
	}
	_ = rows.Close()
	if !found {
		return rs, nil, false, nil
	}
	rules, err := db.ListMaskingRules(ctx, rs.ID)
	return rs, rules, true, err
}

// ListMaskingRules returns the rules of a ruleset in order.
func (db *DB) ListMaskingRules(ctx context.Context, rulesetID string) ([]MaskingRule, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, ruleset_id, target, kind, params, deterministic, salt_ref, position FROM database_masking_rules WHERE ruleset_id = ? ORDER BY position`, rulesetID)
	if err != nil {
		return nil, fmt.Errorf("store: list masking rules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []MaskingRule{}
	for rows.Next() {
		var r MaskingRule
		var det int
		if err := rows.Scan(&r.ID, &r.RulesetID, &r.Target, &r.Kind, &r.Params, &det, &r.SaltRef, &r.Position); err != nil {
			return nil, fmt.Errorf("store: scan masking rule: %w", err)
		}
		r.Deterministic = det == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// ErrMaskingRulesetNotFound is returned when a database has no ruleset of that version.
var ErrMaskingRulesetNotFound = errors.New("store: masking ruleset not found")

// GetMaskingRuleset returns one pinned version of a database's rules.
func (db *DB) GetMaskingRuleset(ctx context.Context, name string, version int) (MaskingRuleset, []MaskingRule, error) {
	var rs MaskingRuleset
	err := db.QueryRowContext(ctx, `SELECT id, database_name, version, created_by, created_at FROM database_masking_rulesets WHERE database_name = ? AND version = ?`, name, version).Scan(&rs.ID, &rs.DatabaseName, &rs.Version, &rs.CreatedBy, &rs.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return rs, nil, ErrMaskingRulesetNotFound
	}
	if err != nil {
		return rs, nil, fmt.Errorf("store: get masking ruleset: %w", err)
	}
	rules, err := db.ListMaskingRules(ctx, rs.ID)
	return rs, rules, err
}
