package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StandaloneDomainOwner is the owner name domainOwner reports for a
// hostname held by a standalone domain.
const StandaloneDomainOwner = "standalone domain"

// ErrStandaloneDomainNotFound is returned when no standalone domain has the name.
var ErrStandaloneDomainNotFound = errors.New("store: standalone domain not found")

// StandaloneDomain is a domain not owned by any app that proxies to Target.
type StandaloneDomain struct {
	Domain             string
	Target             string
	TargetKind         string
	UpstreamTLS        bool
	TLSSkipVerify      bool
	TLSServerName      string
	HostHeader         string
	WebSocket          bool
	DialTimeoutSeconds int
	ReadTimeoutSeconds int
	StripPrefix        string
	Description        string
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

const standaloneDomainColumns = `domain, target, target_kind, upstream_tls, tls_skip_verify, tls_server_name,
	host_header, websocket, dial_timeout_seconds, read_timeout_seconds, strip_prefix, description,
	created_by, created_at, updated_at`

// CreateStandaloneDomain inserts d, refusing a hostname any app, static site
// or other standalone domain already holds (*ErrDomainTaken).
func (db *DB) CreateStandaloneDomain(ctx context.Context, d StandaloneDomain) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create standalone domain: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	owner, found, err := domainOwner(ctx, tx, d.Domain)
	if err != nil {
		return fmt.Errorf("store: create standalone domain %q: %w", d.Domain, err)
	}
	if found {
		return &ErrDomainTaken{Domain: d.Domain, Owner: owner}
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, `INSERT INTO standalone_domains (`+standaloneDomainColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, standaloneDomainArgs(d)...); err != nil {
		return fmt.Errorf("store: create standalone domain %q: %w", d.Domain, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: create standalone domain %q: commit: %w", d.Domain, err)
	}
	return nil
}

// UpdateStandaloneDomain replaces everything but the domain and creation fields.
func (db *DB) UpdateStandaloneDomain(ctx context.Context, d StandaloneDomain) error {
	res, err := db.ExecContext(ctx, `UPDATE standalone_domains SET target = ?, target_kind = ?, upstream_tls = ?,
		tls_skip_verify = ?, tls_server_name = ?, host_header = ?, websocket = ?, dial_timeout_seconds = ?,
		read_timeout_seconds = ?, strip_prefix = ?, description = ?, updated_at = ? WHERE domain = ?`,
		d.Target, d.TargetKind, d.UpstreamTLS, d.TLSSkipVerify, d.TLSServerName, d.HostHeader, d.WebSocket,
		d.DialTimeoutSeconds, d.ReadTimeoutSeconds, d.StripPrefix, d.Description, time.Now().UTC().Format(time.RFC3339Nano), d.Domain)
	if err != nil {
		return fmt.Errorf("store: update standalone domain %q: %w", d.Domain, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStandaloneDomainNotFound
	}
	return nil
}

// GetStandaloneDomain returns one standalone domain or ErrStandaloneDomainNotFound.
func (db *DB) GetStandaloneDomain(ctx context.Context, domain string) (StandaloneDomain, error) {
	row := db.QueryRowContext(ctx, `SELECT `+standaloneDomainColumns+` FROM standalone_domains WHERE domain = ?`, domain)
	d, err := scanStandaloneDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return StandaloneDomain{}, ErrStandaloneDomainNotFound
	}
	if err != nil {
		return StandaloneDomain{}, fmt.Errorf("store: get standalone domain %q: %w", domain, err)
	}
	return d, nil
}

// ListStandaloneDomains returns every standalone domain ordered by domain.
func (db *DB) ListStandaloneDomains(ctx context.Context) ([]StandaloneDomain, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+standaloneDomainColumns+` FROM standalone_domains ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list standalone domains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StandaloneDomain
	for rows.Next() {
		d, err := scanStandaloneDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan standalone domain: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate standalone domains: %w", err)
	}
	return out, nil
}

// DeleteStandaloneDomain removes one standalone domain.
func (db *DB) DeleteStandaloneDomain(ctx context.Context, domain string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM standalone_domains WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete standalone domain %q: %w", domain, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStandaloneDomainNotFound
	}
	return nil
}

func standaloneDomainArgs(d StandaloneDomain) []any {
	return []any{d.Domain, d.Target, d.TargetKind, d.UpstreamTLS, d.TLSSkipVerify, d.TLSServerName,
		d.HostHeader, d.WebSocket, d.DialTimeoutSeconds, d.ReadTimeoutSeconds, d.StripPrefix, d.Description,
		d.CreatedBy, d.CreatedAt.UTC().Format(time.RFC3339Nano), d.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}

func scanStandaloneDomain(r rowScanner) (StandaloneDomain, error) {
	var d StandaloneDomain
	var created, updated string
	if err := r.Scan(&d.Domain, &d.Target, &d.TargetKind, &d.UpstreamTLS, &d.TLSSkipVerify, &d.TLSServerName,
		&d.HostHeader, &d.WebSocket, &d.DialTimeoutSeconds, &d.ReadTimeoutSeconds, &d.StripPrefix, &d.Description,
		&d.CreatedBy, &created, &updated); err != nil {
		return d, err
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return d, nil
}

// ServerAddressOverride is the operator's own public IPv4 and IPv6.
type ServerAddressOverride struct {
	IPv4      string
	IPv6      string
	UpdatedAt time.Time
}

// GetServerAddressOverride returns the override, zero when none is set.
func (db *DB) GetServerAddressOverride(ctx context.Context) (ServerAddressOverride, error) {
	var o ServerAddressOverride
	var updated string
	err := db.QueryRowContext(ctx, `SELECT ipv4, ipv6, updated_at FROM server_address_override WHERE id = 1`).Scan(&o.IPv4, &o.IPv6, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerAddressOverride{}, nil
	}
	if err != nil {
		return o, fmt.Errorf("store: get server address override: %w", err)
	}
	o.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return o, nil
}

// SetServerAddressOverride saves o; both fields empty clears the override.
func (db *DB) SetServerAddressOverride(ctx context.Context, o ServerAddressOverride) error {
	if o.IPv4 == "" && o.IPv6 == "" {
		if _, err := db.ExecContext(ctx, `DELETE FROM server_address_override WHERE id = 1`); err != nil {
			return fmt.Errorf("store: clear server address override: %w", err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, `INSERT INTO server_address_override (id, ipv4, ipv6, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET ipv4 = excluded.ipv4, ipv6 = excluded.ipv6, updated_at = excluded.updated_at`,
		o.IPv4, o.IPv6, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set server address override: %w", err)
	}
	return nil
}
