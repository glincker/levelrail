package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ParkedDomainOwner is the owner name domainOwner reports for a parked hostname.
const ParkedDomainOwner = "parked domain"

var (
	// ErrParkedDomainNotFound is returned when no parked domain has the name.
	ErrParkedDomainNotFound = errors.New("store: parked domain not found")
	// ErrDomainNotOnApp is returned when the app's default domain set lacks the domain.
	ErrDomainNotOnApp = errors.New("store: domain is not in the app's domain list")
	// ErrDomainOnApp is returned when the target app already has the domain.
	ErrDomainOnApp = errors.New("store: domain is already on the app")
)

// ParkedDomain is a domain registered to the instance but routed by no app.
type ParkedDomain struct {
	Domain     string
	ParkedFrom string
	Note       string
	ParkedBy   string
	ParkedAt   time.Time
	// HasSettings is true when the park kept the domain's per-domain settings.
	HasSettings bool
}

// ParkServiceDomain removes domain from app's default domain list and
// records it as parked, in one transaction.
func (db *DB) ParkServiceDomain(ctx context.Context, app, domain, by, note string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: park domain %q: begin: %w", domain, err)
	}
	defer func() { _ = tx.Rollback() }()
	// Releasing the claim deletes the settings, so they are read first.
	snap, err := snapshotDomainSettingsTx(ctx, tx, domain)
	if err != nil {
		return err
	}
	if _, _, err := editServiceDomainsTx(ctx, tx, app, func(cur []string) ([]string, error) {
		if !slices.Contains(cur, domain) {
			return nil, ErrDomainNotOnApp
		}
		return slices.DeleteFunc(cur, func(d string) bool { return d == domain }), nil
	}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO parked_domains (domain, parked_from, note, parked_by, parked_at, settings)
		VALUES (?, ?, ?, ?, ?, ?)`, domain, app, note, by, time.Now().UTC().Format(time.RFC3339Nano), snap); err != nil {
		return fmt.Errorf("store: park domain %q: %w", domain, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: park domain %q: commit: %w", domain, err)
	}
	return nil
}

// AttachParkedDomain gives a parked domain to app and restores its kept
// settings. It returns ErrParkedDomainNotFound, ErrServiceNotFound or
// *ErrDomainTaken.
func (db *DB) AttachParkedDomain(ctx context.Context, domain, app string) error {
	_, err := db.AttachParkedDomainWith(ctx, domain, app, true)
	return err
}

// AttachParkedDomainWith is AttachParkedDomain with a choice: restore false
// drops the kept settings. It returns the settings tables restored.
func (db *DB) AttachParkedDomainWith(ctx context.Context, domain, app string, restore bool) ([]string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: attach domain %q: begin: %w", domain, err)
	}
	defer func() { _ = tx.Rollback() }()
	var snap string
	err = tx.QueryRowContext(ctx, `SELECT settings FROM parked_domains WHERE domain = ?`, domain).Scan(&snap)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrParkedDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: attach domain %q: %w", domain, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM parked_domains WHERE domain = ?`, domain); err != nil {
		return nil, fmt.Errorf("store: attach domain %q: %w", domain, err)
	}
	if _, _, err := editServiceDomainsTx(ctx, tx, app, func(cur []string) ([]string, error) {
		return append(cur, domain), nil
	}); err != nil {
		return nil, err
	}
	var restored []string
	if restore {
		if restored, err = restoreDomainSettingsTx(ctx, tx, domain, snap); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: attach domain %q: commit: %w", domain, err)
	}
	return restored, nil
}

// MoveServiceDomain hands domain from one app to another. The
// service_domains row is re-pointed rather than recreated so per-domain
// settings (they cascade from that row) survive the move.
func (db *DB) MoveServiceDomain(ctx context.Context, domain, from, to string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: move domain %q: begin: %w", domain, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireServiceTx(ctx, tx, to); err != nil {
		return err
	}
	if err := rewriteDomainsTx(ctx, tx, to, func(cur []string) ([]string, error) {
		if slices.Contains(cur, domain) {
			return nil, ErrDomainOnApp
		}
		return append(cur, domain), nil
	}); err != nil {
		return err
	}
	if err := rewriteDomainsTx(ctx, tx, from, func(cur []string) ([]string, error) {
		if !slices.Contains(cur, domain) {
			return nil, ErrDomainNotOnApp
		}
		return slices.DeleteFunc(cur, func(d string) bool { return d == domain }), nil
	}); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE service_domains SET service_name = ? WHERE domain = ? AND service_name = ?`, to, domain, from)
	if err != nil {
		return fmt.Errorf("store: move domain %q: %w", domain, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDomainNotOnApp
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: move domain %q: commit: %w", domain, err)
	}
	return nil
}

// DomainOwnerOf reports which app, static site, standalone domain or park
// holds domain; found is false when it is free.
func (db *DB) DomainOwnerOf(ctx context.Context, domain string) (owner string, found bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("store: domain owner %q: begin: %w", domain, err)
	}
	defer func() { _ = tx.Rollback() }()
	return domainOwner(ctx, tx, domain)
}

// GetParkedDomain returns one parked domain or ErrParkedDomainNotFound.
func (db *DB) GetParkedDomain(ctx context.Context, domain string) (ParkedDomain, error) {
	row := db.QueryRowContext(ctx, `SELECT domain, parked_from, note, parked_by, parked_at, settings <> '' FROM parked_domains WHERE domain = ?`, domain)
	p, err := scanParkedDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ParkedDomain{}, ErrParkedDomainNotFound
	}
	if err != nil {
		return ParkedDomain{}, fmt.Errorf("store: get parked domain %q: %w", domain, err)
	}
	return p, nil
}

// ListParkedDomains returns every parked domain ordered by domain.
func (db *DB) ListParkedDomains(ctx context.Context) ([]ParkedDomain, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain, parked_from, note, parked_by, parked_at, settings <> '' FROM parked_domains ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list parked domains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ParkedDomain
	for rows.Next() {
		p, err := scanParkedDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan parked domain: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate parked domains: %w", err)
	}
	return out, nil
}

// DeleteParkedDomain forgets a parked domain, freeing the hostname.
func (db *DB) DeleteParkedDomain(ctx context.Context, domain string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM parked_domains WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete parked domain %q: %w", domain, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrParkedDomainNotFound
	}
	return nil
}

func scanParkedDomain(r rowScanner) (ParkedDomain, error) {
	var p ParkedDomain
	var at string
	if err := r.Scan(&p.Domain, &p.ParkedFrom, &p.Note, &p.ParkedBy, &at, &p.HasSettings); err != nil {
		return p, err
	}
	p.ParkedAt, _ = time.Parse(time.RFC3339Nano, at)
	return p, nil
}

func requireServiceTx(ctx context.Context, tx *sql.Tx, name string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM desired_services WHERE name = ?`, name).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("store: check app %q: %w", name, err)
	}
	return nil
}

// rewriteDomainsTx writes name's domain JSON without touching service_domains,
// for callers that manage the claim row themselves.
func rewriteDomainsTx(ctx context.Context, tx *sql.Tx, name string, edit func([]string) ([]string, error)) error {
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT domains FROM desired_services WHERE name = ?`, name).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceNotFound
		}
		return fmt.Errorf("store: read domains for %q: %w", name, err)
	}
	var cur []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cur); err != nil {
			return fmt.Errorf("store: decode domains for %q: %w", name, err)
		}
	}
	next, err := edit(cur)
	if err != nil {
		return err
	}
	enc, err := json.Marshal(nonNilSlice(next))
	if err != nil {
		return fmt.Errorf("store: encode domains for %q: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET domains = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(enc), name); err != nil {
		return fmt.Errorf("store: write domains for %q: %w", name, err)
	}
	return nil
}
