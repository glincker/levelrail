package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AuthStore is the store surface the auth handlers need.
type AuthStore interface {
	GetUserByEmail(ctx context.Context, email string) (*store.User, error)
	GetUserByID(ctx context.Context, id string) (*store.User, error)
	CreateUser(ctx context.Context, u store.User) error
	UpdateUserPasswordHash(ctx context.Context, id string, hash *string) error
	UpdateUserLastLogin(ctx context.Context, id string, when time.Time) error
	CountUsers(ctx context.Context) (int, error)
	ListUsers(ctx context.Context) ([]store.User, error)
	DeleteUser(ctx context.Context, id string) error
	UpdateUserAbilities(ctx context.Context, id string, abilities []string) error
	UpdateUserDisplayName(ctx context.Context, id, name string) error
	SetUserSetupSession(ctx context.Context, userID, sessionID, ip string) error
	ClaimFirstLoginTrust(ctx context.Context, userID string) (bool, error)
}

// SessionLinkTokenStore is the store surface the session-link flow
// needs: always set, part of the core Store interface, same shape as
// PasswordResetTokenStore above.
type SessionLinkTokenStore interface {
	SaveSessionLinkToken(ctx context.Context, t store.SessionLinkToken) error
	GetSessionLinkTokenByHash(ctx context.Context, hash string) (*store.SessionLinkToken, error)
	ClaimSessionLinkToken(ctx context.Context, id string) error
}

// InviteStore is the store surface the team-invite flow needs: always
// set, part of the core Store interface, same shape as
// PasswordResetTokenStore above.
type InviteStore interface {
	SaveInvite(ctx context.Context, inv store.Invite) error
	GetInviteByHash(ctx context.Context, hash string) (*store.Invite, error)
	GetInviteByID(ctx context.Context, id string) (*store.Invite, error)
	ListPendingInvites(ctx context.Context) ([]store.Invite, error)
	RevokeInvite(ctx context.Context, id string) error
	ClaimInvite(ctx context.Context, id string) error
}

// TokenStore is the store surface the API-token handlers and the
// ability-aware auth middleware need.
type TokenStore interface {
	SaveAPIToken(ctx context.Context, t store.APIToken) error
	GetAPITokenByHash(ctx context.Context, hash string) (*store.APIToken, error)
	GetAPITokenByID(ctx context.Context, id string) (*store.APIToken, error)
	ListAPITokens(ctx context.Context) ([]store.APIToken, error)
	RevokeAPIToken(ctx context.Context, id string) error
	TouchAPITokenLastUsed(ctx context.Context, id string) error
}

// OnboardingStore is defined in onboarding.go, next to the handlers that
// use it (the same "single-file feature" shape scheduled_task.go/
// feature_flags.go already use, rather than every store interface living
// in this file).

// AuditStore is the store surface requireAbility's audit hook
// (internal/api/auth.go), GET /api/v1/audit-log (audit.go), and the
// retention sweep/manual purge (audit_retention.go) need: insert-only
// recording, newest-first cursor-paginated reading, and age-based
// deletion. Always set, the same "core Store interface, not an optional
// plug-in" shape certs/staticSites/backupTargets already have.
type AuditStore interface {
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
	ListAuditEntries(ctx context.Context, limit int, before *time.Time, filter store.AuditEntryFilter) ([]store.AuditEntry, error)
	DeleteAuditEntriesOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}
