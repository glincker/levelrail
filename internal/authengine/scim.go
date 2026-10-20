package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
)

const libSCIMPrefix = "/scim/v2"

// ErrSCIMTokenNotFound means no token of the instance organization has that id.
var ErrSCIMTokenNotFound = errors.New("authengine: scim token not found")

// SCIMToken is a directory sync token. Only its sha256 hash is stored.
type SCIMToken struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

func toSCIMToken(t theauth.SCIMToken) SCIMToken {
	return SCIMToken{ID: t.ID.String(), Name: t.Name, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt}
}

// CreateSCIMToken mints a token for the instance organization. The plaintext
// is returned once and never stored.
func (e *Engine) CreateSCIMToken(ctx context.Context, name string) (string, SCIMToken, error) {
	plain, rec, err := e.auth.CreateSCIMToken(ctx, e.orgID, name)
	if err != nil {
		return "", SCIMToken{}, fmt.Errorf("authengine: create scim token: %w", err)
	}
	return plain, toSCIMToken(rec), nil
}

// ListSCIMTokens returns every token of the instance organization, revoked ones included.
func (e *Engine) ListSCIMTokens(ctx context.Context) ([]SCIMToken, error) {
	rows, err := e.auth.ListSCIMTokens(ctx, e.orgID)
	if err != nil {
		return nil, fmt.Errorf("authengine: list scim tokens: %w", err)
	}
	out := make([]SCIMToken, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSCIMToken(r))
	}
	return out, nil
}

// SCIMTokenByID finds one token of the instance organization.
func (e *Engine) SCIMTokenByID(ctx context.Context, id string) (SCIMToken, error) {
	all, err := e.ListSCIMTokens(ctx)
	if err != nil {
		return SCIMToken{}, err
	}
	for _, t := range all {
		if t.ID == id {
			return t, nil
		}
	}
	return SCIMToken{}, ErrSCIMTokenNotFound
}

// RevokeSCIMToken revokes one token of the instance organization.
func (e *Engine) RevokeSCIMToken(ctx context.Context, id string) error {
	if _, err := e.SCIMTokenByID(ctx, id); err != nil {
		return err
	}
	uid, err := parseULID(id)
	if err != nil {
		return ErrSCIMTokenNotFound
	}
	if err := e.auth.RevokeSCIMToken(ctx, uid); err != nil {
		return fmt.Errorf("authengine: revoke scim token: %w", err)
	}
	return nil
}

// AuthenticateSCIMToken resolves a presented bearer token to its token id. It
// is false for an unknown, revoked or foreign-organization token.
func (e *Engine) AuthenticateSCIMToken(ctx context.Context, presented string) (string, bool, error) {
	res, err := e.auth.AuthenticateSCIMToken(ctx, presented)
	if errors.Is(err, theauth.ErrInvalidToken) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("authengine: authenticate scim token: %w", err)
	}
	if res.OrgID != e.orgID {
		return "", false, nil
	}
	return res.TokenID.String(), true, nil
}

// SCIMHandler serves the library's SCIM 2.0 endpoints under mountPrefix
// (for example /api/v1/scim/v2). Anything else under it is a 404.
func (e *Engine) SCIMHandler(mountPrefix string) http.Handler {
	var once sync.Once
	var lib http.Handler
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { lib = e.auth.Handler() })
		rest, ok := strings.CutPrefix(r.URL.Path, mountPrefix)
		if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = libSCIMPrefix + rest
		r2.URL.RawPath = ""
		lib.ServeHTTP(w, r2)
	})
}

// SCIMLocationBase is the URL prefix the library writes into meta.location.
func (e *Engine) SCIMLocationBase() string { return e.baseURL + libSCIMPrefix }

// DirectoryUser is the library's record of a user the directory manages.
type DirectoryUser struct {
	EngineID    string
	Email       string
	DisplayName string
	Member      bool
	Groups      []string
}

// DirectoryUser reads a library user, whether it is still a member of the
// instance organization, and the SCIM groups it belongs to.
func (e *Engine) DirectoryUser(ctx context.Context, engineID string) (DirectoryUser, bool, error) {
	u := DirectoryUser{EngineID: engineID}
	err := e.db.QueryRowContext(ctx, `SELECT email, COALESCE(NULLIF(display_name, ''), name) FROM theauth_users WHERE id = ?`, engineID).
		Scan(&u.Email, &u.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return DirectoryUser{}, false, nil
	}
	if err != nil {
		return DirectoryUser{}, false, fmt.Errorf("authengine: directory user: %w", err)
	}
	var n int
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM theauth_organization_members WHERE organization_id = ? AND user_id = ?`,
		e.orgID.String(), engineID).Scan(&n); err != nil {
		return DirectoryUser{}, false, fmt.Errorf("authengine: directory membership: %w", err)
	}
	u.Member = n > 0
	rows, err := e.db.QueryContext(ctx, `
		SELECT g.display_name FROM theauth_group_members gm JOIN theauth_groups g ON g.id = gm.group_id
		WHERE gm.user_id = ? AND g.organization_id = ? ORDER BY g.display_name`, engineID, e.orgID.String())
	if err != nil {
		return DirectoryUser{}, false, fmt.Errorf("authengine: directory groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return DirectoryUser{}, false, fmt.Errorf("authengine: directory groups: %w", err)
		}
		u.Groups = append(u.Groups, g)
	}
	return u, true, rows.Err()
}

// GroupMemberIDs lists the library user ids in a SCIM group of the instance
// organization; an unknown group has none.
func (e *Engine) GroupMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT gm.user_id FROM theauth_group_members gm JOIN theauth_groups g ON g.id = gm.group_id
		WHERE gm.group_id = ? AND g.organization_id = ? ORDER BY gm.user_id`, groupID, e.orgID.String())
	if err != nil {
		return nil, fmt.Errorf("authengine: group members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("authengine: group members: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AdoptForSCIM lets the library's own idempotent create pick up an existing
// account: it records externalID on the library user and makes it a member.
// A user already bound to a different external id is left alone.
func (e *Engine) AdoptForSCIM(ctx context.Context, engineID, externalID string) (bool, error) {
	if externalID == "" {
		return false, nil
	}
	id, err := parseULID(engineID)
	if err != nil {
		return false, err
	}
	u, err := e.store.UserByID(ctx, id)
	if err != nil {
		return false, fmt.Errorf("authengine: adopt for scim: %w", err)
	}
	if u.ExternalID != "" && u.ExternalID != externalID {
		return false, nil
	}
	u.ExternalID = externalID
	if err := e.store.UpdateUserSCIM(ctx, *u); err != nil {
		return false, fmt.Errorf("authengine: adopt for scim: %w", err)
	}
	if err := e.auth.AddOrganizationMember(ctx, e.orgID, id, "member"); err != nil {
		return false, fmt.Errorf("authengine: adopt for scim: %w", err)
	}
	return true, nil
}

// SetDirectoryMember adds or removes a library user from the instance organization.
func (e *Engine) SetDirectoryMember(ctx context.Context, engineID string, member bool) error {
	id, err := parseULID(engineID)
	if err != nil {
		return err
	}
	if member {
		err = e.auth.AddOrganizationMember(ctx, e.orgID, id, "member")
	} else {
		err = e.auth.RemoveOrganizationMember(ctx, e.orgID, id)
		if errors.Is(err, theauth.ErrStorageNotFound) {
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("authengine: directory membership: %w", err)
	}
	return nil
}

// DirectoryMembers lists the library ids of every member of the instance organization.
func (e *Engine) DirectoryMembers(ctx context.Context) ([]string, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT user_id FROM theauth_organization_members WHERE organization_id = ? ORDER BY user_id`, e.orgID.String())
	if err != nil {
		return nil, fmt.Errorf("authengine: directory members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("authengine: directory members: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
