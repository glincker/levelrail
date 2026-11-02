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
	// ParkedFromEnvironment is the environment ID the domain left, empty for the app's default list.
	ParkedFromEnvironment string
	// ParkedFromKind is DomainKindApp or DomainKindStaticSite.
	ParkedFromKind string
	Note           string
	ParkedBy       string
	ParkedAt       time.Time
	// HasSettings is true when the park kept the domain's per-domain settings.
	HasSettings bool
}

// ParkServiceDomain removes domain from app's default domain list and
// records it as parked, in one transaction.
func (db *DB) ParkServiceDomain(ctx context.Context, app, domain, by, note string) error {
	return db.ParkServiceDomainInEnv(ctx, app, "", domain, by, note)
}

// ParkServiceDomainInEnv is ParkServiceDomain for the domain set of envID;
// an empty envID is the app's default list.
func (db *DB) ParkServiceDomainInEnv(ctx context.Context, app, envID, domain, by, note string) error {
	return db.ParkDomainFrom(ctx, DomainTarget{Kind: DomainKindApp, Name: app, EnvID: envID}, domain, by, note)
}

// ParkDomainFrom parks domain out of src, an app set or a static site. Only
// app domains have per-domain settings to keep.
func (db *DB) ParkDomainFrom(ctx context.Context, src DomainTarget, domain, by, note string) error {
	src.Kind = kindOrApp(src.Kind)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: park domain %q: begin: %w", domain, err)
	}
	defer func() { _ = tx.Rollback() }()
	var snap string
	if !src.isStatic() {
		// Releasing the claim deletes the settings, so they are read first.
		if snap, err = snapshotDomainSettingsTx(ctx, tx, domain); err != nil {
			return err
		}
	}
	if err := editTargetClaimTx(ctx, tx, src, removeDomainEdit(domain)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO parked_domains (domain, parked_from, parked_from_environment, parked_from_kind, note, parked_by, parked_at, settings)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, domain, src.Name, src.EnvID, src.Kind, note, by, time.Now().UTC().Format(time.RFC3339Nano), snap); err != nil {
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
	return db.AttachParkedDomainInEnv(ctx, domain, app, "", restore)
}

// AttachParkedDomainInEnv is AttachParkedDomainWith into the domain set of
// envID; an empty envID is the app's default list. It also returns
// ErrEnvironmentNotFound.
func (db *DB) AttachParkedDomainInEnv(ctx context.Context, domain, app, envID string, restore bool) ([]string, error) {
	return db.AttachParkedDomainTo(ctx, domain, DomainTarget{Kind: DomainKindApp, Name: app, EnvID: envID}, restore)
}

// AttachParkedDomainTo attaches a parked domain to dst, an app set or a
// static site. Kept settings are restored only onto an app.
func (db *DB) AttachParkedDomainTo(ctx context.Context, domain string, dst DomainTarget, restore bool) ([]string, error) {
	dst.Kind = kindOrApp(dst.Kind)
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
	if err := editTargetClaimTx(ctx, tx, dst, func(cur []string) ([]string, error) {
		return append(cur, domain), nil
	}); err != nil {
		return nil, err
	}
	var restored []string
	if restore && !dst.isStatic() {
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
	return db.MoveServiceDomainBetween(ctx, domain, from, "", to, "")
}

// MoveServiceDomainBetween is MoveServiceDomain between domain sets: an
// empty environment ID is that app's default list. Moving between two
// environments of one app is allowed.
func (db *DB) MoveServiceDomainBetween(ctx context.Context, domain, from, fromEnv, to, toEnv string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: move domain %q: begin: %w", domain, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireServiceTx(ctx, tx, to); err != nil {
		return err
	}
	if err := rewriteDomainSetTx(ctx, tx, to, toEnv, func(cur []string) ([]string, error) {
		if slices.Contains(cur, domain) {
			return nil, ErrDomainOnApp
		}
		return append(cur, domain), nil
	}); err != nil {
		return err
	}
	if err := rewriteDomainSetTx(ctx, tx, from, fromEnv, func(cur []string) ([]string, error) {
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
	row := db.QueryRowContext(ctx, `SELECT domain, parked_from, parked_from_environment, parked_from_kind, note, parked_by, parked_at, settings <> '' FROM parked_domains WHERE domain = ?`, domain)
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
	rows, err := db.QueryContext(ctx, `SELECT domain, parked_from, parked_from_environment, parked_from_kind, note, parked_by, parked_at, settings <> '' FROM parked_domains ORDER BY domain`)
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
	if err := r.Scan(&p.Domain, &p.ParkedFrom, &p.ParkedFromEnvironment, &p.ParkedFromKind, &p.Note, &p.ParkedBy, &at, &p.HasSettings); err != nil {
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

func requireEnvironmentTx(ctx context.Context, tx *sql.Tx, id string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM environments WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEnvironmentNotFound
	}
	if err != nil {
		return fmt.Errorf("store: check environment %q: %w", id, err)
	}
	return nil
}

// editDomainSetClaimTx edits one domain set and re-syncs the app's claims,
// so a removed domain is released and an added one is collision-checked.
func editDomainSetClaimTx(ctx context.Context, tx *sql.Tx, app, envID string, edit func([]string) ([]string, error)) error {
	if envID == "" {
		_, _, err := editServiceDomainsTx(ctx, tx, app, edit)
		return err
	}
	if err := requireServiceTx(ctx, tx, app); err != nil {
		return err
	}
	if err := requireEnvironmentTx(ctx, tx, envID); err != nil {
		return err
	}
	cur, err := envDomainsTx(ctx, tx, app, envID)
	if err != nil {
		return err
	}
	next, err := edit(slices.Clone(cur))
	if err != nil {
		return err
	}
	if err := replaceEnvDomainsTx(ctx, tx, app, envID, nonNilSlice(next)); err != nil {
		return err
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT domains FROM desired_services WHERE name = ?`, app).Scan(&raw); err != nil {
		return fmt.Errorf("store: read domains for %q: %w", app, err)
	}
	var base []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &base); err != nil {
			return fmt.Errorf("store: decode domains for %q: %w", app, err)
		}
	}
	if err := claimServiceDomains(ctx, tx, app, base); err != nil {
		return fmt.Errorf("store: edit environment domains for %q: %w", app, err)
	}
	return nil
}

// rewriteDomainSetTx edits one domain set without touching service_domains.
func rewriteDomainSetTx(ctx context.Context, tx *sql.Tx, app, envID string, edit func([]string) ([]string, error)) error {
	if envID == "" {
		return rewriteDomainsTx(ctx, tx, app, edit)
	}
	if err := requireServiceTx(ctx, tx, app); err != nil {
		return err
	}
	if err := requireEnvironmentTx(ctx, tx, envID); err != nil {
		return err
	}
	cur, err := envDomainsTx(ctx, tx, app, envID)
	if err != nil {
		return err
	}
	next, err := edit(slices.Clone(cur))
	if err != nil {
		return err
	}
	return replaceEnvDomainsTx(ctx, tx, app, envID, nonNilSlice(next))
}
