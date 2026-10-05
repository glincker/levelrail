package authengine

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/glincker/theauth-go/v2/crypto"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ResolveOAuthUser returns the platform user id behind a library OAuth account.
// A user the library created on first sign-in gets a platform user with the
// default read ability, and the identity is mirrored so rolling back keeps it.
func (e *Engine) ResolveOAuthUser(ctx context.Context, provider, providerUserID string) (string, error) {
	rt := e.oauth
	if rt == nil {
		return "", errors.New("authengine: oauth is not enabled")
	}
	var engineID string
	err := rt.db.QueryRowContext(ctx, `
		SELECT user_id FROM theauth_oauth_accounts WHERE provider = ? AND provider_user_id = ?`,
		provider, providerUserID).Scan(&engineID)
	if err != nil {
		return "", fmt.Errorf("authengine: find oauth account: %w", err)
	}
	legacyID, err := rt.legacyUserFor(ctx, engineID)
	if err != nil {
		return "", err
	}
	identityID, err := randomID("oid_")
	if err != nil {
		return "", err
	}
	err = rt.wiring.Users.SaveOAuthIdentity(ctx, store.OAuthIdentity{
		ID: identityID, UserID: legacyID, Provider: provider, ProviderUserID: providerUserID, CreatedAt: time.Now(),
	})
	if err != nil && !errors.Is(err, store.ErrOAuthIdentityAlreadyLinked) {
		return "", fmt.Errorf("authengine: mirror oauth identity: %w", err)
	}
	return legacyID, nil
}

func (rt *oauthRuntime) legacyUserFor(ctx context.Context, engineID string) (string, error) {
	var legacyID string
	err := rt.db.QueryRowContext(ctx, `SELECT legacy_id FROM authengine_user_map WHERE engine_id = ?`, engineID).Scan(&legacyID)
	if err == nil {
		return legacyID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("authengine: look up user map: %w", err)
	}
	var email, name, display sql.NullString
	err = rt.db.QueryRowContext(ctx, `SELECT email, name, display_name FROM theauth_users WHERE id = ?`, engineID).Scan(&email, &name, &display)
	if err != nil {
		return "", fmt.Errorf("authengine: load library user %s: %w", engineID, err)
	}
	addr := strings.ToLower(strings.TrimSpace(email.String))
	if addr == "" {
		return "", fmt.Errorf("authengine: library user %s has no email", engineID)
	}
	label := display.String
	if label == "" {
		label = name.String
	}
	if label == "" {
		label = addr
	}
	legacyID, err = randomID("user_")
	if err != nil {
		return "", err
	}
	if err := rt.wiring.Users.CreateUser(ctx, store.User{
		ID: legacyID, Email: addr, DisplayName: label, Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		return "", fmt.Errorf("authengine: create platform user for library user %s: %w", engineID, err)
	}
	if _, err := rt.db.ExecContext(ctx, `INSERT INTO authengine_user_map (legacy_id, engine_id) VALUES (?, ?)`, legacyID, engineID); err != nil {
		return "", fmt.Errorf("authengine: map user %s: %w", legacyID, err)
	}
	return legacyID, nil
}

func randomID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("authengine: generate %sid: %w", prefix, err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// LinkOAuthIdentity mirrors an identity linked through the in-house link flow into the
// library, so a provider linked after cutover still signs in. Users not yet copied are
// skipped: the backfill carries their identities.
func (e *Engine) LinkOAuthIdentity(ctx context.Context, legacyUserID, provider, providerUserID string) error {
	rt := e.oauth
	if rt == nil {
		return nil
	}
	var engineID string
	err := rt.db.QueryRowContext(ctx, `SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, legacyUserID).Scan(&engineID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("authengine: look up user map: %w", err)
	}
	placeholder, err := crypto.Encrypt(rt.key, []byte{})
	if err != nil {
		return fmt.Errorf("authengine: encrypt oauth placeholder token: %w", err)
	}
	now := time.Now().UTC().UnixMicro()
	if _, err := rt.db.ExecContext(ctx, `
		INSERT INTO theauth_oauth_accounts (id, user_id, provider, provider_user_id, access_token_enc,
			refresh_token_enc, expires_at, scope, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NULL, NULL, '', ?, ?)
		ON CONFLICT (provider, provider_user_id) DO NOTHING`,
		ulid.Make().String(), engineID, provider, providerUserID, placeholder, now, now); err != nil {
		return fmt.Errorf("authengine: mirror linked identity: %w", err)
	}
	return nil
}
