package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Providers that hold several connections each.
const (
	GitProviderGitLab    = "gitlab"
	GitProviderBitbucket = "bitbucket"
	GitProviderGitea     = "gitea"

	// GitProviderForgejo is a Forgejo instance, reached through the Gitea-compatible API.
	GitProviderForgejo = "forgejo"
	// GitProviderBitbucketServer is Bitbucket Server or Data Center, authenticated with a personal access token.
	GitProviderBitbucketServer = "bitbucket_server"
)

// ErrGitConnectionNotFound is returned when a connection id matches no row.
var ErrGitConnectionNotFound = errors.New("store: git connection not found")

// GitConnection is one OAuth application plus the account authorized
// through it. Credentials (client secret, tokens) live in internal/secrets
// under SecretsKey, never in this row.
type GitConnection struct {
	ID          string
	Provider    string
	Label       string
	Host        string
	InstanceURL string
	ClientID    string
	SecretsKey  string
	AccountID   string
	AccountName string
	Scopes      string
	IsDefault   bool
	CreatedBy   string
	CreatedAt   string
}

// NewGitConnectionID returns a fresh connection id.
func NewGitConnectionID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: generate git connection id: %w", err)
	}
	return "gc_" + hex.EncodeToString(b), nil
}

// GitConnectionSecretsKey is the internal/secrets serviceName for a
// connection created after multi-account support. Migrated connections
// keep the key their single row already used.
func GitConnectionSecretsKey(id string) string { return "git-conn/" + id }

// RepoHost is the lowercase hostname of a repository URL, without port or credentials.
func RepoHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

const gitConnColumns = `id, provider, label, host, instance_url, client_id, secrets_key, account_id, account_name, scopes, is_default, created_by, created_at`

func scanGitConnection(scan func(dest ...any) error) (GitConnection, error) {
	var c GitConnection
	var def int
	err := scan(&c.ID, &c.Provider, &c.Label, &c.Host, &c.InstanceURL, &c.ClientID, &c.SecretsKey, &c.AccountID, &c.AccountName, &c.Scopes, &def, &c.CreatedBy, &c.CreatedAt)
	c.IsDefault = def != 0
	return c, err
}

// CreateGitConnection inserts a connection. The provider's first connection
// becomes its default.
func (db *DB) CreateGitConnection(ctx context.Context, c GitConnection) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM git_provider_connections WHERE provider = ?`, c.Provider).Scan(&n); err != nil {
		return fmt.Errorf("store: count %s connections: %w", c.Provider, err)
	}
	c.IsDefault = n == 0
	def := 0
	if c.IsDefault {
		def = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO git_provider_connections (`+gitConnColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Provider, c.Label, strings.ToLower(c.Host), c.InstanceURL, c.ClientID, c.SecretsKey, c.AccountID, c.AccountName, c.Scopes, def, c.CreatedBy, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: create git connection: %w", err)
	}
	return nil
}

// GetGitConnection returns one connection by id.
func (db *DB) GetGitConnection(ctx context.Context, id string) (GitConnection, error) {
	c, err := scanGitConnection(db.QueryRowContext(ctx, `SELECT `+gitConnColumns+` FROM git_provider_connections WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return GitConnection{}, ErrGitConnectionNotFound
	}
	if err != nil {
		return GitConnection{}, fmt.Errorf("store: get git connection %q: %w", id, err)
	}
	return c, nil
}

// ListGitConnections returns connections oldest first; an empty provider lists every provider.
func (db *DB) ListGitConnections(ctx context.Context, provider string) ([]GitConnection, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+gitConnColumns+` FROM git_provider_connections WHERE (? = '' OR provider = ?) ORDER BY created_at, rowid`, provider, provider)
	if err != nil {
		return nil, fmt.Errorf("store: list git connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []GitConnection
	for rows.Next() {
		c, err := scanGitConnection(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: list git connections: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list git connections: %w", err)
	}
	return out, nil
}

// FindGitConnectionByAccount returns the connection for the same remote
// account on the same host authorized through the same OAuth application.
func (db *DB) FindGitConnectionByAccount(ctx context.Context, provider, host, accountID, clientID, excludeID string) (GitConnection, bool, error) {
	if accountID == "" {
		return GitConnection{}, false, nil
	}
	c, err := scanGitConnection(db.QueryRowContext(ctx, `SELECT `+gitConnColumns+` FROM git_provider_connections
		WHERE provider = ? AND host = ? AND account_id = ? AND client_id = ? AND id != ? LIMIT 1`,
		provider, strings.ToLower(host), accountID, clientID, excludeID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return GitConnection{}, false, nil
	}
	if err != nil {
		return GitConnection{}, false, fmt.Errorf("store: find git connection by account: %w", err)
	}
	return c, true, nil
}

// RenameGitConnection sets a connection's display label.
func (db *DB) RenameGitConnection(ctx context.Context, id, label string) error {
	return db.execGitConn(ctx, id, `UPDATE git_provider_connections SET label = ? WHERE id = ?`, label, id)
}

// SetGitConnectionAccount records the authenticated account behind a connection.
func (db *DB) SetGitConnectionAccount(ctx context.Context, id, accountID, accountName, scopes string) error {
	return db.execGitConn(ctx, id, `UPDATE git_provider_connections SET account_id = ?, account_name = ?, scopes = ? WHERE id = ?`, accountID, accountName, scopes, id)
}

// SetGitConnectionOAuthApp replaces a connection's OAuth application, used
// when the same account is authorized again through a newly entered app.
func (db *DB) SetGitConnectionOAuthApp(ctx context.Context, id, instanceURL, clientID string) error {
	return db.execGitConn(ctx, id, `UPDATE git_provider_connections SET instance_url = ?, client_id = ? WHERE id = ?`, instanceURL, clientID, id)
}

func (db *DB) execGitConn(ctx context.Context, id, query string, args ...any) error {
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store: update git connection %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrGitConnectionNotFound
	}
	return nil
}

// SetDefaultGitConnection makes id the default of its provider.
func (db *DB) SetDefaultGitConnection(ctx context.Context, id string) error {
	c, err := db.GetGitConnection(ctx, id)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set default git connection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE git_provider_connections SET is_default = (id = ?) WHERE provider = ?`, id, c.Provider); err != nil {
		return fmt.Errorf("store: set default git connection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set default git connection: %w", err)
	}
	return nil
}

// DeleteGitConnection removes a connection and, when it was the default,
// promotes the oldest remaining connection of the same provider. Sources
// that referenced it keep their recorded id.
func (db *DB) DeleteGitConnection(ctx context.Context, id string) error {
	c, err := db.GetGitConnection(ctx, id)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete git connection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM git_provider_connections WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete git connection %q: %w", id, err)
	}
	if c.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE git_provider_connections SET is_default = 1 WHERE id = (
			SELECT id FROM git_provider_connections WHERE provider = ? ORDER BY created_at, rowid LIMIT 1)`, c.Provider); err != nil {
			return fmt.Errorf("store: promote default git connection: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete git connection %q: %w", id, err)
	}
	return nil
}
