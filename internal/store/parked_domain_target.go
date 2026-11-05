package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const (
	// DomainKindApp is a container app's domain list.
	DomainKindApp = "app"
	// DomainKindStaticSite is a static site's domain list.
	DomainKindStaticSite = "static_site"
)

// ErrStaticSiteEnvironment is returned when an environment is named for a static site.
var ErrStaticSiteEnvironment = errors.New("store: static sites have no environment domain sets")

// DomainTarget names one domain set: an app (default list or one
// environment's set) or a static site (always its single list).
type DomainTarget struct {
	Kind  string
	Name  string
	EnvID string
}

func (t DomainTarget) isStatic() bool { return t.Kind == DomainKindStaticSite }

// kindOrApp keeps rows and callers that never set a kind meaning an app.
func kindOrApp(kind string) string {
	if kind == "" {
		return DomainKindApp
	}
	return kind
}

// editTargetClaimTx edits one domain set and re-syncs its claims.
func editTargetClaimTx(ctx context.Context, tx *sql.Tx, t DomainTarget, edit func([]string) ([]string, error)) error {
	if !t.isStatic() {
		return editDomainSetClaimTx(ctx, tx, t.Name, t.EnvID, edit)
	}
	if t.EnvID != "" {
		return ErrStaticSiteEnvironment
	}
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT domains FROM static_sites WHERE name = ?`, t.Name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("store: read domains for static site %q: %w", t.Name, err)
	}
	cur, err := decodeDomainList(raw)
	if err != nil {
		return fmt.Errorf("store: decode domains for static site %q: %w", t.Name, err)
	}
	next, err := edit(slices.Clone(cur))
	if err != nil {
		return err
	}
	enc, err := json.Marshal(nonNilSlice(next))
	if err != nil {
		return fmt.Errorf("store: encode domains for static site %q: %w", t.Name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE static_sites SET domains = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(enc), t.Name); err != nil {
		return fmt.Errorf("store: write domains for static site %q: %w", t.Name, err)
	}
	if err := claimStaticSiteDomains(ctx, tx, t.Name, next); err != nil {
		return fmt.Errorf("store: edit domains for static site %q: %w", t.Name, err)
	}
	return nil
}

func removeDomainEdit(domain string) func([]string) ([]string, error) {
	return func(cur []string) ([]string, error) {
		if !slices.Contains(cur, domain) {
			return nil, ErrDomainNotOnApp
		}
		return slices.DeleteFunc(cur, func(d string) bool { return d == domain }), nil
	}
}

// MoveDomainBetweenTargets hands domain from one target to another. App to
// app keeps per-domain settings; a move that touches a static site drops
// them, because only app domains have settings tables.
func (db *DB) MoveDomainBetweenTargets(ctx context.Context, domain string, from, to DomainTarget) error {
	if !from.isStatic() && !to.isStatic() {
		return db.MoveServiceDomainBetween(ctx, domain, from.Name, from.EnvID, to.Name, to.EnvID)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: move domain %q: begin: %w", domain, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := editTargetClaimTx(ctx, tx, from, removeDomainEdit(domain)); err != nil {
		return err
	}
	if err := editTargetClaimTx(ctx, tx, to, func(cur []string) ([]string, error) {
		if slices.Contains(cur, domain) {
			return nil, ErrDomainOnApp
		}
		return append(cur, domain), nil
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: move domain %q: commit: %w", domain, err)
	}
	return nil
}
