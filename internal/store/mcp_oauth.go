package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrMCPOAuthNotFound is returned when a client, request or grant does not exist.
var ErrMCPOAuthNotFound = errors.New("store: mcp oauth record not found")

// MCPOAuthClient is a pre-registered public OAuth client.
type MCPOAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
	Scopes       []string
	CreatedBy    string
	CreatedAt    int64
}

// MCPOAuthRequest is one authorization request waiting for, or past, consent.
type MCPOAuthRequest struct {
	ID            string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Scopes        []string
	Resource      string
	State         string
	ExpiresAt     int64
	UserID        string
	Decided       bool
	Approved      bool
}

// MCPOAuthGrant is the standing authorization behind a family of tokens.
type MCPOAuthGrant struct {
	ID        string
	ClientID  string
	UserID    string
	Scopes    []string
	Resource  string
	RequestID string
	ExpiresAt int64
	Revoked   bool
}

// MCPRefreshOutcome says what a refresh token presentation found.
type MCPRefreshOutcome int

// Refresh token presentation results.
const (
	MCPRefreshUnknown MCPRefreshOutcome = iota
	MCPRefreshOK
	MCPRefreshReused
	MCPRefreshExpired
)

// CreateMCPOAuthClient stores a pre-registered client.
func (db *DB) CreateMCPOAuthClient(ctx context.Context, c MCPOAuthClient) error {
	uris, _ := json.Marshal(nonNilSlice(c.RedirectURIs))
	scopes, _ := json.Marshal(nonNilSlice(c.Scopes))
	_, err := db.ExecContext(ctx, `INSERT INTO mcp_oauth_clients (id, name, redirect_uris, scopes, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, string(uris), string(scopes), c.CreatedBy, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: create mcp oauth client: %w", err)
	}
	return nil
}

func scanMCPClient(scan func(...any) error) (MCPOAuthClient, error) {
	var c MCPOAuthClient
	var uris, scopes string
	if err := scan(&c.ID, &c.Name, &uris, &scopes, &c.CreatedBy, &c.CreatedAt); err != nil {
		return c, err
	}
	_ = json.Unmarshal([]byte(uris), &c.RedirectURIs)
	_ = json.Unmarshal([]byte(scopes), &c.Scopes)
	return c, nil
}

const mcpClientCols = `id, name, redirect_uris, scopes, created_by, created_at`

// GetMCPOAuthClient returns one client or ErrMCPOAuthNotFound.
func (db *DB) GetMCPOAuthClient(ctx context.Context, id string) (*MCPOAuthClient, error) {
	c, err := scanMCPClient(db.QueryRowContext(ctx, `SELECT `+mcpClientCols+` FROM mcp_oauth_clients WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMCPOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get mcp oauth client: %w", err)
	}
	return &c, nil
}

// ListMCPOAuthClients returns every client, newest first.
func (db *DB) ListMCPOAuthClients(ctx context.Context) ([]MCPOAuthClient, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+mcpClientCols+` FROM mcp_oauth_clients ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list mcp oauth clients: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MCPOAuthClient
	for rows.Next() {
		c, err := scanMCPClient(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan mcp oauth client: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteMCPOAuthClient removes the client and revokes its grants, returning
// the access token ids that must be revoked in the token store.
func (db *DB) DeleteMCPOAuthClient(ctx context.Context, id string, now int64) ([]string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: delete mcp oauth client: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM mcp_oauth_clients WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: delete mcp oauth client: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrMCPOAuthNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT t.token_id FROM mcp_oauth_tokens t JOIN mcp_oauth_grants g ON g.id = t.grant_id WHERE g.client_id = ? AND g.revoked_at IS NULL`, id)
	if err != nil {
		return nil, fmt.Errorf("store: delete mcp oauth client tokens: %w", err)
	}
	var ids []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: scan mcp oauth token: %w", err)
		}
		ids = append(ids, tid)
	}
	_ = rows.Close()
	if _, err := tx.ExecContext(ctx, `UPDATE mcp_oauth_grants SET revoked_at = ? WHERE client_id = ? AND revoked_at IS NULL`, now, id); err != nil {
		return nil, fmt.Errorf("store: revoke mcp oauth client grants: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: delete mcp oauth client: %w", err)
	}
	return ids, nil
}

// CreateMCPOAuthRequest parks a validated authorization request.
func (db *DB) CreateMCPOAuthRequest(ctx context.Context, r MCPOAuthRequest, now int64) error {
	scopes, _ := json.Marshal(nonNilSlice(r.Scopes))
	_, err := db.ExecContext(ctx, `INSERT INTO mcp_oauth_requests (id, client_id, redirect_uri, code_challenge, scopes, resource, state, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ClientID, r.RedirectURI, r.CodeChallenge, string(scopes), r.Resource, r.State, now, r.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store: create mcp oauth request: %w", err)
	}
	return nil
}

// GetMCPOAuthRequest returns one authorization request.
func (db *DB) GetMCPOAuthRequest(ctx context.Context, id string) (*MCPOAuthRequest, error) {
	var r MCPOAuthRequest
	var scopes string
	var decided sql.NullInt64
	var approved int
	err := db.QueryRowContext(ctx, `SELECT id, client_id, redirect_uri, code_challenge, scopes, resource, state, expires_at, user_id, decided_at, approved FROM mcp_oauth_requests WHERE id = ?`, id).
		Scan(&r.ID, &r.ClientID, &r.RedirectURI, &r.CodeChallenge, &scopes, &r.Resource, &r.State, &r.ExpiresAt, &r.UserID, &decided, &approved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMCPOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get mcp oauth request: %w", err)
	}
	_ = json.Unmarshal([]byte(scopes), &r.Scopes)
	r.Decided, r.Approved = decided.Valid, approved == 1
	return &r, nil
}

// DecideMCPOAuthRequest records the user's decision once. It reports false
// when the request was already decided or has expired.
func (db *DB) DecideMCPOAuthRequest(ctx context.Context, id, userID string, approved bool, scopes []string, codeHash string, codeExpires, now int64) (bool, error) {
	sj, _ := json.Marshal(nonNilSlice(scopes))
	ap := 0
	if approved {
		ap = 1
	}
	res, err := db.ExecContext(ctx, `UPDATE mcp_oauth_requests SET user_id = ?, decided_at = ?, approved = ?, scopes = ?, code_hash = ?, code_expires_at = ? WHERE id = ? AND decided_at IS NULL AND expires_at > ?`,
		userID, now, ap, string(sj), codeHash, codeExpires, id, now)
	if err != nil {
		return false, fmt.Errorf("store: decide mcp oauth request: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// GetMCPOAuthRequestByCode loads the approved request a code hash belongs to
// without consuming it, so the caller can check the presenting client first.
func (db *DB) GetMCPOAuthRequestByCode(ctx context.Context, codeHash string) (*MCPOAuthRequest, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM mcp_oauth_requests WHERE code_hash = ? AND code_hash <> '' AND approved = 1`, codeHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMCPOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: lookup mcp oauth code: %w", err)
	}
	return db.GetMCPOAuthRequest(ctx, id)
}

// ConsumeMCPOAuthCode marks the code used exactly once. reused is true when
// the code was already redeemed, with the request returned so its grant can be revoked.
func (db *DB) ConsumeMCPOAuthCode(ctx context.Context, codeHash string, now int64) (req *MCPOAuthRequest, reused bool, err error) {
	var id string
	var used sql.NullInt64
	var exp int64
	var approved int
	err = db.QueryRowContext(ctx, `SELECT id, code_used_at, code_expires_at, approved FROM mcp_oauth_requests WHERE code_hash = ? AND code_hash <> ''`, codeHash).Scan(&id, &used, &exp, &approved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrMCPOAuthNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: lookup mcp oauth code: %w", err)
	}
	if approved != 1 {
		return nil, false, ErrMCPOAuthNotFound
	}
	res, err := db.ExecContext(ctx, `UPDATE mcp_oauth_requests SET code_used_at = ? WHERE id = ? AND code_used_at IS NULL AND code_expires_at > ?`, now, id, now)
	if err != nil {
		return nil, false, fmt.Errorf("store: consume mcp oauth code: %w", err)
	}
	n, _ := res.RowsAffected()
	r, gerr := db.GetMCPOAuthRequest(ctx, id)
	if gerr != nil {
		return nil, false, gerr
	}
	if n == 1 {
		return r, false, nil
	}
	return r, used.Valid, ErrMCPOAuthNotFound
}

// CreateMCPOAuthGrant stores a grant with its first access token and refresh hash.
func (db *DB) CreateMCPOAuthGrant(ctx context.Context, g MCPOAuthGrant, tokenID, refreshHash string, refreshExpires, now int64) error {
	sj, _ := json.Marshal(nonNilSlice(g.Scopes))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create mcp oauth grant: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_oauth_grants (id, client_id, user_id, scopes, resource, request_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.ClientID, g.UserID, string(sj), g.Resource, g.RequestID, now, g.ExpiresAt); err != nil {
		return fmt.Errorf("store: create mcp oauth grant: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_oauth_tokens (token_id, grant_id, created_at) VALUES (?, ?, ?)`, tokenID, g.ID, now); err != nil {
		return fmt.Errorf("store: create mcp oauth grant token: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_oauth_refresh (hash, grant_id, expires_at) VALUES (?, ?, ?)`, refreshHash, g.ID, refreshExpires); err != nil {
		return fmt.Errorf("store: create mcp oauth grant refresh: %w", err)
	}
	return tx.Commit()
}

func (db *DB) getMCPGrant(ctx context.Context, where string, arg any) (*MCPOAuthGrant, error) {
	var g MCPOAuthGrant
	var scopes string
	var revoked sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT id, client_id, user_id, scopes, resource, request_id, expires_at, revoked_at FROM mcp_oauth_grants WHERE `+where, arg).
		Scan(&g.ID, &g.ClientID, &g.UserID, &scopes, &g.Resource, &g.RequestID, &g.ExpiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMCPOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get mcp oauth grant: %w", err)
	}
	_ = json.Unmarshal([]byte(scopes), &g.Scopes)
	g.Revoked = revoked.Valid
	return &g, nil
}

// GetMCPOAuthGrant returns one grant.
func (db *DB) GetMCPOAuthGrant(ctx context.Context, id string) (*MCPOAuthGrant, error) {
	return db.getMCPGrant(ctx, "id = ?", id)
}

// GetMCPOAuthGrantByToken returns the grant an access token belongs to, or
// ErrMCPOAuthNotFound for a token that OAuth never issued.
func (db *DB) GetMCPOAuthGrantByToken(ctx context.Context, tokenID string) (*MCPOAuthGrant, error) {
	return db.getMCPGrant(ctx, "id = (SELECT grant_id FROM mcp_oauth_tokens WHERE token_id = ?)", tokenID)
}

// GetMCPOAuthGrantByRequest returns the grant created from an authorization request.
func (db *DB) GetMCPOAuthGrantByRequest(ctx context.Context, requestID string) (*MCPOAuthGrant, error) {
	return db.getMCPGrant(ctx, "request_id = ?", requestID)
}

// RotateMCPOAuthRefresh spends the presented refresh hash and stores its
// successor. A hash that was already spent reports MCPRefreshReused.
func (db *DB) RotateMCPOAuthRefresh(ctx context.Context, oldHash, newHash string, newExpires int64, newTokenID string, now int64) (MCPRefreshOutcome, string, error) {
	var grantID string
	var exp int64
	var used sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT grant_id, expires_at, used_at FROM mcp_oauth_refresh WHERE hash = ?`, oldHash).Scan(&grantID, &exp, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return MCPRefreshUnknown, "", nil
	}
	if err != nil {
		return MCPRefreshUnknown, "", fmt.Errorf("store: lookup mcp oauth refresh: %w", err)
	}
	if used.Valid {
		return MCPRefreshReused, grantID, nil
	}
	if exp <= now {
		return MCPRefreshExpired, grantID, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return MCPRefreshUnknown, "", fmt.Errorf("store: rotate mcp oauth refresh: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE mcp_oauth_refresh SET used_at = ? WHERE hash = ? AND used_at IS NULL`, now, oldHash)
	if err != nil {
		return MCPRefreshUnknown, "", fmt.Errorf("store: rotate mcp oauth refresh: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return MCPRefreshReused, grantID, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_oauth_refresh (hash, grant_id, expires_at) VALUES (?, ?, ?)`, newHash, grantID, newExpires); err != nil {
		return MCPRefreshUnknown, "", fmt.Errorf("store: rotate mcp oauth refresh: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_oauth_tokens (token_id, grant_id, created_at) VALUES (?, ?, ?)`, newTokenID, grantID, now); err != nil {
		return MCPRefreshUnknown, "", fmt.Errorf("store: rotate mcp oauth refresh token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MCPRefreshUnknown, "", fmt.Errorf("store: rotate mcp oauth refresh: %w", err)
	}
	return MCPRefreshOK, grantID, nil
}

// RevokeMCPOAuthGrant marks the grant revoked and returns every access token
// id it issued so the caller can revoke them in the token store.
func (db *DB) RevokeMCPOAuthGrant(ctx context.Context, grantID string, now int64) ([]string, error) {
	if _, err := db.ExecContext(ctx, `UPDATE mcp_oauth_grants SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, grantID); err != nil {
		return nil, fmt.Errorf("store: revoke mcp oauth grant: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT token_id FROM mcp_oauth_tokens WHERE grant_id = ?`, grantID)
	if err != nil {
		return nil, fmt.Errorf("store: list mcp oauth grant tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan mcp oauth grant token: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MCPOAuthGrantTokenIDs lists every access token id issued under a grant.
func (db *DB) MCPOAuthGrantTokenIDs(ctx context.Context, grantID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT token_id FROM mcp_oauth_tokens WHERE grant_id = ?`, grantID)
	if err != nil {
		return nil, fmt.Errorf("store: list mcp oauth grant tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan mcp oauth grant token: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetMCPOAuthGrantByRefresh returns the grant a refresh token hash belongs to.
func (db *DB) GetMCPOAuthGrantByRefresh(ctx context.Context, hash string) (*MCPOAuthGrant, error) {
	return db.getMCPGrant(ctx, "id = (SELECT grant_id FROM mcp_oauth_refresh WHERE hash = ?)", hash)
}
