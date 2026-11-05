package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrInvalidOrgSlug is returned for a slug that breaks the format or is reserved.
	ErrInvalidOrgSlug = errors.New("store: invalid organization slug")
	// ErrOrgSlugTaken is returned when another organization already uses the slug.
	ErrOrgSlugTaken = errors.New("store: organization slug is taken")
	// ErrOrgAvatarNotFound is returned when an organization has no avatar.
	ErrOrgAvatarNotFound = errors.New("store: organization has no avatar")
)

const (
	orgSlugMin = 3
	orgSlugMax = 40
)

var orgSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedOrgSlugs are names that would shadow a route, the default team or the reserved project.
var reservedOrgSlugs = map[string]bool{
	"default": true, "admin": true, "api": true, "login": true, "settings": true,
	"global": true, "organizations": true, "teams": true, "new": true, "static": true,
	"assets": true, "www": true, "logout": true, "signup": true,
}

// ValidateOrgSlug checks the slug format and the reserved names.
func ValidateOrgSlug(slug string) error {
	if len(slug) < orgSlugMin || len(slug) > orgSlugMax {
		return fmt.Errorf("%w: use %d to %d characters", ErrInvalidOrgSlug, orgSlugMin, orgSlugMax)
	}
	if !orgSlugPattern.MatchString(slug) {
		return fmt.Errorf("%w: use lowercase letters, digits and single hyphens", ErrInvalidOrgSlug)
	}
	if reservedOrgSlugs[slug] {
		return fmt.Errorf("%w: %q is reserved", ErrInvalidOrgSlug, slug)
	}
	return nil
}

// OrgProfilePatch carries the profile fields to change; nil leaves a field alone and
// an empty Slug clears it.
type OrgProfilePatch struct {
	Name        *string
	Slug        *string
	Description *string
}

// UpdateOrganizationProfile applies a patch and returns the stored organization.
func (db *DB) UpdateOrganizationProfile(ctx context.Context, id string, p OrgProfilePatch) (Organization, error) {
	cur, err := db.GetOrganization(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	if p.Name != nil {
		cur.Name = *p.Name
	}
	if p.Description != nil {
		cur.Description = *p.Description
	}
	if p.Slug != nil {
		cur.Slug = strings.ToLower(*p.Slug)
		if cur.Slug != "" {
			if id == DefaultTeamID {
				return Organization{}, fmt.Errorf("%w: the default organization has no slug", ErrInvalidOrgSlug)
			}
			if err := ValidateOrgSlug(cur.Slug); err != nil {
				return Organization{}, err
			}
		}
	}
	cur.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
		UPDATE organizations SET name = ?, slug = ?, description = ?, updated_at = ? WHERE id = ?`,
		cur.Name, sql.NullString{String: cur.Slug, Valid: cur.Slug != ""}, cur.Description, cur.UpdatedAt, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Organization{}, ErrOrgSlugTaken
		}
		return Organization{}, fmt.Errorf("store: update organization %q: %w", id, err)
	}
	return cur, nil
}

// OrgAvatar is the stored, already re-encoded avatar of an organization.
type OrgAvatar struct {
	Mime      string
	Bytes     []byte
	SHA       string
	UpdatedAt string
}

// SetOrganizationAvatar stores the avatar and records its hash on the organization.
func (db *DB) SetOrganizationAvatar(ctx context.Context, orgID string, a OrgAvatar) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set organization avatar: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO organization_avatars (org_id, mime, bytes, sha, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(org_id) DO UPDATE SET mime = excluded.mime, bytes = excluded.bytes, sha = excluded.sha, updated_at = excluded.updated_at`,
		orgID, a.Mime, a.Bytes, a.SHA, now); err != nil {
		return fmt.Errorf("store: set organization avatar: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organizations SET avatar_sha = ?, updated_at = ? WHERE id = ?`, a.SHA, now, orgID); err != nil {
		return fmt.Errorf("store: set organization avatar: %w", err)
	}
	return tx.Commit()
}

// GetOrganizationAvatar returns the avatar or ErrOrgAvatarNotFound.
func (db *DB) GetOrganizationAvatar(ctx context.Context, orgID string) (OrgAvatar, error) {
	var a OrgAvatar
	err := db.QueryRowContext(ctx, `SELECT mime, bytes, sha, updated_at FROM organization_avatars WHERE org_id = ?`, orgID).
		Scan(&a.Mime, &a.Bytes, &a.SHA, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgAvatar{}, ErrOrgAvatarNotFound
	}
	if err != nil {
		return OrgAvatar{}, fmt.Errorf("store: get organization avatar: %w", err)
	}
	return a, nil
}

// DeleteOrganizationAvatar removes the avatar, or returns ErrOrgAvatarNotFound.
func (db *DB) DeleteOrganizationAvatar(ctx context.Context, orgID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete organization avatar: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM organization_avatars WHERE org_id = ?`, orgID)
	if err != nil {
		return fmt.Errorf("store: delete organization avatar: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrOrgAvatarNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organizations SET avatar_sha = '', updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), orgID); err != nil {
		return fmt.Errorf("store: delete organization avatar: %w", err)
	}
	return tx.Commit()
}

// OrgCounts are the tile numbers of an organization home.
type OrgCounts struct {
	Projects       int
	Apps           int
	Databases      int
	Environments   int
	Members        int
	PendingInvites int
}

// OrganizationCounts counts what one organization owns, in one statement.
func (db *DB) OrganizationCounts(ctx context.Context, orgID string, now time.Time) (OrgCounts, error) {
	var c OrgCounts
	err := db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM projects WHERE org_id = ?1),
		  (SELECT COUNT(*) FROM desired_services s JOIN projects p ON p.id = s.project_id WHERE p.org_id = ?1 AND s.parent_service = ''),
		  (SELECT COUNT(*) FROM desired_databases d JOIN projects p ON p.id = d.project_id WHERE p.org_id = ?1),
		  (SELECT COUNT(*) FROM environments e JOIN projects p ON p.id = e.project_id WHERE p.org_id = ?1),
		  (SELECT COUNT(*) FROM team_members WHERE team_id = ?1),
		  (SELECT COUNT(*) FROM team_invites WHERE team_id = ?1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?2)`,
		orgID, formatTime(now)).Scan(&c.Projects, &c.Apps, &c.Databases, &c.Environments, &c.Members, &c.PendingInvites)
	if err != nil {
		return OrgCounts{}, fmt.Errorf("store: organization counts %q: %w", orgID, err)
	}
	return c, nil
}

// OrganizationFailingApps lists apps of the organization whose newest deploy failed, at most limit.
func (db *DB) OrganizationFailingApps(ctx context.Context, orgID string, limit int) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.name FROM desired_services s
		JOIN projects p ON p.id = s.project_id
		JOIN deploy_attempts a ON a.id = (
			SELECT id FROM deploy_attempts WHERE service_name = s.name ORDER BY started_at DESC LIMIT 1)
		WHERE p.org_id = ? AND s.parent_service = '' AND a.status = ?
		ORDER BY s.name LIMIT ?`, orgID, DeployAttemptStatusFailed, limit)
	if err != nil {
		return nil, fmt.Errorf("store: organization failing apps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scan failing app: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListTeamEventsPage returns events older than the (createdAt, id) cursor, newest first.
// An empty cursor starts from the newest.
func (db *DB) ListTeamEventsPage(ctx context.Context, teamID, curCreated, curID string, limit int) ([]TeamEvent, error) {
	q := `SELECT id, team_id, actor_id, actor_name, action, subject, detail, created_at FROM team_events WHERE team_id = ?`
	args := []any{teamID}
	if curCreated != "" {
		q += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, curCreated, curCreated, curID)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list team events page: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TeamEvent
	for rows.Next() {
		var e TeamEvent
		var created string
		if err := rows.Scan(&e.ID, &e.TeamID, &e.ActorID, &e.ActorName, &e.Action, &e.Subject, &e.Detail, &created); err != nil {
			return nil, fmt.Errorf("store: scan team event: %w", err)
		}
		if e.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse team event time: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
