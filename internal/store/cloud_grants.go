package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Actions a grant may carry. Build and delete are deliberately absent: they
// stay root only until nodes can be pinned to a team.
const (
	CloudActionView         = "view"
	CloudActionOperate      = "operate"
	CloudActionManageBudget = "manage_budget"
	CloudActionViewSpend    = "view_spend"
	CloudActionShare        = "share"
)

// Scope levels a grant attaches to, widest first.
const (
	CloudResourceAccount     = "account"
	CloudResourceCompartment = "compartment"
	CloudResourceInstance    = "instance"
)

// Principal kinds a grant can name.
const (
	CloudPrincipalUser = "user"
	CloudPrincipalTeam = "team"
)

// CloudGrantRootPrefix marks grants written by root so no-escalation checks know the origin.
const CloudGrantRootPrefix = "root:"

// ErrCloudGrantNotFound is returned when a grant id matches no row.
var ErrCloudGrantNotFound = errors.New("store: cloud grant not found")

// CloudGrantActions lists every grantable action in display order.
func CloudGrantActions() []string {
	return []string{CloudActionView, CloudActionOperate, CloudActionManageBudget, CloudActionViewSpend, CloudActionShare}
}

// CloudGrant gives one principal a set of actions on an account, a compartment
// or one instance of it. A nil ExpiresAt never expires.
type CloudGrant struct {
	ID            string
	AccountID     string
	ResourceKind  string
	ResourceRef   string
	PrincipalType string
	PrincipalID   string
	Actions       []string
	GrantedBy     string
	ExpiresAt     *time.Time
	CreatedAt     time.Time
}

// CloudPrincipal names who is asking: a user id or a team id.
type CloudPrincipal struct {
	Type string
	ID   string
}

// ValidateCloudGrant checks kinds, ref shape and that every action is grantable.
func ValidateCloudGrant(g CloudGrant) error {
	switch g.ResourceKind {
	case CloudResourceAccount:
		if g.ResourceRef != "" {
			return errors.New("store: an account grant has no resource ref")
		}
	case CloudResourceCompartment, CloudResourceInstance:
		if strings.TrimSpace(g.ResourceRef) == "" {
			return fmt.Errorf("store: a %s grant needs a resource ref", g.ResourceKind)
		}
	default:
		return fmt.Errorf("store: unknown cloud grant resource kind %q", g.ResourceKind)
	}
	if g.PrincipalType != CloudPrincipalUser && g.PrincipalType != CloudPrincipalTeam {
		return fmt.Errorf("store: unknown cloud grant principal type %q", g.PrincipalType)
	}
	if strings.TrimSpace(g.PrincipalID) == "" || strings.TrimSpace(g.AccountID) == "" {
		return errors.New("store: a cloud grant needs an account and a principal")
	}
	if len(g.Actions) == 0 {
		return errors.New("store: a cloud grant needs at least one action")
	}
	valid := CloudGrantActions()
	for _, a := range g.Actions {
		if !slices.Contains(valid, a) {
			return fmt.Errorf("store: action %q cannot be granted", a)
		}
	}
	return nil
}

// UpsertCloudGrant creates a grant or replaces the actions and expiry of the
// one already on the same account, scope and principal.
func (db *DB) UpsertCloudGrant(ctx context.Context, g CloudGrant) error {
	if err := ValidateCloudGrant(g); err != nil {
		return err
	}
	actions, err := json.Marshal(slices.Compact(slices.Sorted(slices.Values(g.Actions))))
	if err != nil {
		return fmt.Errorf("store: encode grant actions: %w", err)
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO cloud_grants (id, account_id, resource_kind, resource_ref, principal_type, principal_id, actions, granted_by, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (account_id, resource_kind, resource_ref, principal_type, principal_id)
		DO UPDATE SET actions = excluded.actions, granted_by = excluded.granted_by, expires_at = excluded.expires_at`,
		g.ID, g.AccountID, g.ResourceKind, g.ResourceRef, g.PrincipalType, g.PrincipalID, string(actions), g.GrantedBy,
		cloudGrantExpiry(g.ExpiresAt), formatTime(g.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: upsert cloud grant on %q: %w", g.AccountID, err)
	}
	return nil
}

// ListCloudGrants returns every grant on one account, oldest first.
func (db *DB) ListCloudGrants(ctx context.Context, accountID string) ([]CloudGrant, error) {
	return db.queryCloudGrants(ctx, `WHERE account_id = ?`, accountID)
}

// ListCloudGrantsForPrincipals returns the grants any of the principals hold,
// across all accounts when accountID is empty. Expired rows are included so
// the pure authz check owns the expiry decision.
func (db *DB) ListCloudGrantsForPrincipals(ctx context.Context, accountID string, principals []CloudPrincipal) ([]CloudGrant, error) {
	if len(principals) == 0 {
		return []CloudGrant{}, nil
	}
	conds := make([]string, 0, len(principals))
	var args []any
	for _, p := range principals {
		conds = append(conds, `(principal_type = ? AND principal_id = ?)`)
		args = append(args, p.Type, p.ID)
	}
	where := `WHERE (` + strings.Join(conds, ` OR `) + `)`
	if accountID != "" {
		where += ` AND account_id = ?`
		args = append(args, accountID)
	}
	return db.queryCloudGrants(ctx, where, args...)
}

// DeleteCloudGrant removes one grant by id.
func (db *DB) DeleteCloudGrant(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM cloud_grants WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete cloud grant %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrCloudGrantNotFound, "delete cloud grant %q", id)
}

// DeleteCloudGrantsForPrincipal drops every grant a removed user or team held.
func (db *DB) DeleteCloudGrantsForPrincipal(ctx context.Context, principalType, principalID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM cloud_grants WHERE principal_type = ? AND principal_id = ?`, principalType, principalID); err != nil {
		return fmt.Errorf("store: delete grants of %s %q: %w", principalType, principalID, err)
	}
	return nil
}

func (db *DB) queryCloudGrants(ctx context.Context, where string, args ...any) ([]CloudGrant, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, account_id, resource_kind, resource_ref, principal_type, principal_id, actions, granted_by, expires_at, created_at
		FROM cloud_grants `+where+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list cloud grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []CloudGrant{}
	for rows.Next() {
		g, err := scanCloudGrant(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan cloud grant: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate cloud grants: %w", err)
	}
	return out, nil
}

func cloudGrantExpiry(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}

func scanCloudGrant(scan func(dest ...any) error) (CloudGrant, error) {
	var (
		g                         CloudGrant
		actions, expires, created string
	)
	if err := scan(&g.ID, &g.AccountID, &g.ResourceKind, &g.ResourceRef, &g.PrincipalType, &g.PrincipalID,
		&actions, &g.GrantedBy, &expires, &created); err != nil {
		return CloudGrant{}, err
	}
	if err := json.Unmarshal([]byte(actions), &g.Actions); err != nil {
		return CloudGrant{}, fmt.Errorf("decode actions: %w", err)
	}
	var err error
	if expires != "" {
		t, perr := time.Parse(time.RFC3339Nano, expires)
		if perr != nil {
			return CloudGrant{}, fmt.Errorf("parse expires_at: %w", perr)
		}
		g.ExpiresAt = &t
	}
	if g.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return CloudGrant{}, fmt.Errorf("parse created_at: %w", err)
	}
	return g, nil
}
