package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Branch deploy statuses reuse the preview vocabulary.
const (
	BranchDeployDeploying = PreviewStatusDeploying
	BranchDeployActive    = PreviewStatusActive
	BranchDeployFailed    = PreviewStatusFailed
)

// ErrBranchDeployNotFound is returned when no branch deploy row matches.
var ErrBranchDeployNotFound = errors.New("store: branch deploy not found")

// BranchDeploySettings is one app's branch deploy policy
// (migrations/2550_branch_deploys.sql). Mode is off, all or patterns.
type BranchDeploySettings struct {
	AppName   string
	Mode      string
	Patterns  []string
	UpdatedAt string
}

// BranchDeploy is one live branch deployment: a service named DeployAppID
// built from Branch at HeadSHA and served at Domain.
type BranchDeploy struct {
	ID           string
	AppName      string
	Branch       string
	Slug         string
	DeployAppID  string
	HeadSHA      string
	HeadMessage  string
	Domain       string
	Status       string
	StatusReason string
	CreatedAt    string
	UpdatedAt    string
}

// NewBranchDeployID mints a random branch deploy ID.
func NewBranchDeployID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate branch deploy id: %w", err)
	}
	return "brd_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// GetBranchDeploySettings returns the app's policy, or an off policy when none is saved.
func (db *DB) GetBranchDeploySettings(ctx context.Context, appName string) (BranchDeploySettings, error) {
	var s BranchDeploySettings
	var patterns string
	err := db.QueryRowContext(ctx, `SELECT app_name, mode, patterns, updated_at FROM branch_deploy_settings WHERE app_name = ?`, appName).
		Scan(&s.AppName, &s.Mode, &patterns, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BranchDeploySettings{AppName: appName, Mode: "off", Patterns: []string{}}, nil
	}
	if err != nil {
		return BranchDeploySettings{}, fmt.Errorf("store: get branch deploy settings for %q: %w", appName, err)
	}
	if err := json.Unmarshal([]byte(patterns), &s.Patterns); err != nil {
		return BranchDeploySettings{}, fmt.Errorf("store: decode branch deploy patterns for %q: %w", appName, err)
	}
	if s.Patterns == nil {
		s.Patterns = []string{}
	}
	return s, nil
}

// SaveBranchDeploySettings upserts the app's policy.
func (db *DB) SaveBranchDeploySettings(ctx context.Context, s BranchDeploySettings) error {
	patterns, err := json.Marshal(s.Patterns)
	if err != nil {
		return fmt.Errorf("store: encode branch deploy patterns: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO branch_deploy_settings (app_name, mode, patterns, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET mode = excluded.mode, patterns = excluded.patterns, updated_at = excluded.updated_at
	`, s.AppName, s.Mode, string(patterns), s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save branch deploy settings for %q: %w", s.AppName, err)
	}
	return nil
}

const branchDeployColumns = "id, app_name, branch, slug, deploy_app_id, head_sha, head_message, domain, status, status_reason, created_at, updated_at"

func scanBranchDeploy(scan func(dest ...any) error) (*BranchDeploy, error) {
	var b BranchDeploy
	if err := scan(&b.ID, &b.AppName, &b.Branch, &b.Slug, &b.DeployAppID, &b.HeadSHA, &b.HeadMessage, &b.Domain, &b.Status, &b.StatusReason, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

// SaveBranchDeploy inserts a new branch deploy row.
func (db *DB) SaveBranchDeploy(ctx context.Context, b BranchDeploy) error {
	_, err := db.ExecContext(ctx, `INSERT INTO branch_deploys (`+branchDeployColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.AppName, b.Branch, b.Slug, b.DeployAppID, b.HeadSHA, b.HeadMessage, b.Domain, b.Status, b.StatusReason, b.CreatedAt, b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save branch deploy %q: %w", b.ID, err)
	}
	return nil
}

// UpdateBranchDeploy rewrites the mutable columns of an existing row.
func (db *DB) UpdateBranchDeploy(ctx context.Context, b BranchDeploy) error {
	res, err := db.ExecContext(ctx, `
		UPDATE branch_deploys SET head_sha = ?, head_message = ?, domain = ?, status = ?, status_reason = ?, updated_at = ? WHERE id = ?
	`, b.HeadSHA, b.HeadMessage, b.Domain, b.Status, b.StatusReason, b.UpdatedAt, b.ID)
	if err != nil {
		return fmt.Errorf("store: update branch deploy %q: %w", b.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBranchDeployNotFound
	}
	return nil
}

// GetBranchDeployBySlug returns the row for (appName, slug).
func (db *DB) GetBranchDeployBySlug(ctx context.Context, appName, slug string) (*BranchDeploy, error) {
	b, err := scanBranchDeploy(db.QueryRowContext(ctx, `SELECT `+branchDeployColumns+` FROM branch_deploys WHERE app_name = ? AND slug = ?`, appName, slug).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBranchDeployNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get branch deploy %q/%q: %w", appName, slug, err)
	}
	return b, nil
}

// ListBranchDeploysByApp returns an app's branch deploys, newest update first.
func (db *DB) ListBranchDeploysByApp(ctx context.Context, appName string) ([]BranchDeploy, error) {
	return db.queryBranchDeploys(ctx, `SELECT `+branchDeployColumns+` FROM branch_deploys WHERE app_name = ? ORDER BY updated_at DESC`, appName)
}

// ListBranchDeploys returns every branch deploy across apps.
func (db *DB) ListBranchDeploys(ctx context.Context) ([]BranchDeploy, error) {
	return db.queryBranchDeploys(ctx, `SELECT `+branchDeployColumns+` FROM branch_deploys ORDER BY updated_at ASC`)
}

func (db *DB) queryBranchDeploys(ctx context.Context, query string, args ...any) ([]BranchDeploy, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list branch deploys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BranchDeploy
	for rows.Next() {
		b, err := scanBranchDeploy(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan branch deploy: %w", err)
		}
		out = append(out, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate branch deploys: %w", err)
	}
	return out, nil
}

// DeleteBranchDeploy removes a row once its resources are gone.
func (db *DB) DeleteBranchDeploy(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM branch_deploys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete branch deploy %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBranchDeployNotFound
	}
	return nil
}
