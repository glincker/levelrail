package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// Silence mutes one status issue. A nil Until means forever.
type Silence struct {
	ID        string     `json:"id"`
	IssueKey  string     `json:"issue_key"`
	Scope     string     `json:"scope,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
	Severity  string     `json:"severity,omitempty"`
	CreatedBy string     `json:"created_by"`
	Reason    string     `json:"reason,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Active reports whether the silence still mutes its issue at now.
func (s Silence) Active(now time.Time) bool {
	return s.Until == nil || now.Before(*s.Until)
}

// Matches reports whether the silence covers the issue key in the given
// scope. An unscoped silence covers the key in every scope.
func (s Silence) Matches(issueKey, scope string) bool {
	return s.IssueKey == issueKey && (s.Scope == "" || s.Scope == scope)
}

// FindSilence returns the active silence covering the issue, if any.
func FindSilence(silences []Silence, issueKey, scope string, now time.Time) (Silence, bool) {
	for _, s := range silences {
		if s.Active(now) && s.Matches(issueKey, scope) {
			return s, true
		}
	}
	return Silence{}, false
}

// NewSilenceID mints an opaque silence id.
func NewSilenceID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate silence id: %w", err)
	}
	return "sil_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// CreateSilence stores a silence.
func (db *DB) CreateSilence(ctx context.Context, s Silence) error {
	var until any
	if s.Until != nil {
		until = s.Until.UTC().Format(time.RFC3339)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO silences (id, issue_key, scope, until, severity, created_by, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.IssueKey, s.Scope, until, s.Severity, s.CreatedBy, s.Reason, s.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: create silence for %q: %w", s.IssueKey, err)
	}
	return nil
}

// ListSilences returns every silence, expired ones included: expiry is
// decided by the reader.
func (db *DB) ListSilences(ctx context.Context) ([]Silence, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, issue_key, scope, until, severity, created_by, reason, created_at FROM silences ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list silences: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Silence{}
	for rows.Next() {
		var s Silence
		var until sql.NullString
		var created string
		if err := rows.Scan(&s.ID, &s.IssueKey, &s.Scope, &until, &s.Severity, &s.CreatedBy, &s.Reason, &created); err != nil {
			return nil, fmt.Errorf("store: scan silence: %w", err)
		}
		if until.Valid {
			t, err := time.Parse(time.RFC3339, until.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse silence %q until: %w", s.ID, err)
			}
			s.Until = &t
		}
		if s.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
			return nil, fmt.Errorf("store: parse silence %q created_at: %w", s.ID, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list silences: %w", err)
	}
	return out, nil
}

// ErrSilenceNotFound means no silence has the id.
var ErrSilenceNotFound = errors.New("store: silence not found")

// DeleteSilence removes a silence so its issue shows again.
func (db *DB) DeleteSilence(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM silences WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete silence %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSilenceNotFound
	}
	return nil
}
