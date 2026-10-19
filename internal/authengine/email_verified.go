package authengine

import (
	"context"
	"strings"
)

// EmailVerified reports whether the library user with this email has a verified address.
// Accounts that only ever signed in with a password have none, so this is false for them.
func (e *Engine) EmailVerified(ctx context.Context, email string) bool {
	if e == nil || e.db == nil {
		return false
	}
	var verified bool
	err := e.db.QueryRowContext(ctx,
		`SELECT email_verified_at IS NOT NULL FROM theauth_users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&verified)
	return err == nil && verified
}
