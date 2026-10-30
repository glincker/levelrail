package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Mail ownership errors.
var (
	ErrMailDomainNotFound = errors.New("store: mail domain not found")
	ErrMailDomainTaken    = errors.New("store: mail domain already belongs to a team")
	ErrMailboxNotFound    = errors.New("store: mailbox not found")
	ErrMailboxExists      = errors.New("store: mailbox already exists")
)

// MailDomain is a mail domain one team owns.
type MailDomain struct {
	Domain    string `json:"domain"`
	Service   string `json:"service"`
	TeamID    string `json:"team_id"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// Mailbox is a mailbox's ownership record. It holds no password and no mail.
type Mailbox struct {
	Address      string `json:"address"`
	Domain       string `json:"domain"`
	Localpart    string `json:"localpart"`
	Service      string `json:"service"`
	TeamID       string `json:"team_id"`
	LinkedUserID string `json:"linked_user_id,omitempty"`
	Suspended    bool   `json:"suspended"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ClaimMailDomain records the owning team; a domain another team owns is refused.
func (db *DB) ClaimMailDomain(ctx context.Context, d MailDomain) error {
	d.Domain = strings.ToLower(d.Domain)
	if d.CreatedAt == "" {
		d.CreatedAt = mailNow()
	}
	_, err := db.ExecContext(ctx, `INSERT INTO mail_domains (domain, service, team_id, created_by, created_at) VALUES (?, ?, ?, ?, ?)`,
		d.Domain, d.Service, d.TeamID, d.CreatedBy, d.CreatedAt)
	if isUniqueErr(err) {
		return ErrMailDomainTaken
	}
	return err
}

// GetMailDomain returns one owned domain.
func (db *DB) GetMailDomain(ctx context.Context, domain string) (MailDomain, error) {
	var d MailDomain
	err := db.QueryRowContext(ctx, `SELECT domain, service, team_id, created_by, created_at FROM mail_domains WHERE domain = ?`, strings.ToLower(domain)).
		Scan(&d.Domain, &d.Service, &d.TeamID, &d.CreatedBy, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrMailDomainNotFound
	}
	return d, err
}

// ListMailDomains returns every owned domain; callers filter by team.
func (db *DB) ListMailDomains(ctx context.Context) ([]MailDomain, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain, service, team_id, created_by, created_at FROM mail_domains ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list mail domains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []MailDomain{}
	for rows.Next() {
		var d MailDomain
		if err := rows.Scan(&d.Domain, &d.Service, &d.TeamID, &d.CreatedBy, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

const mailboxCols = `address, domain, localpart, service, team_id, linked_user_id, suspended, created_by, created_at, updated_at`

func scanMailbox(sc interface{ Scan(...any) error }) (Mailbox, error) {
	var m Mailbox
	var susp int
	err := sc.Scan(&m.Address, &m.Domain, &m.Localpart, &m.Service, &m.TeamID, &m.LinkedUserID, &susp, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
	m.Suspended = susp != 0
	return m, err
}

// CreateMailbox records a mailbox under its domain's team.
func (db *DB) CreateMailbox(ctx context.Context, m Mailbox) error {
	now := mailNow()
	m.Address, m.Domain = strings.ToLower(m.Address), strings.ToLower(m.Domain)
	_, err := db.ExecContext(ctx, `INSERT INTO mail_mailboxes (`+mailboxCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.Address, m.Domain, m.Localpart, m.Service, m.TeamID, m.LinkedUserID, boolInt(m.Suspended), m.CreatedBy, now, now)
	if isUniqueErr(err) {
		return ErrMailboxExists
	}
	return err
}

// GetMailbox returns one mailbox record.
func (db *DB) GetMailbox(ctx context.Context, address string) (Mailbox, error) {
	m, err := scanMailbox(db.QueryRowContext(ctx, `SELECT `+mailboxCols+` FROM mail_mailboxes WHERE address = ?`, strings.ToLower(address)))
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrMailboxNotFound
	}
	return m, err
}

// ListMailboxes returns mailboxes, all of them when domain is "".
func (db *DB) ListMailboxes(ctx context.Context, domain string) ([]Mailbox, error) {
	q, args := `SELECT `+mailboxCols+` FROM mail_mailboxes`, []any{}
	if domain != "" {
		q, args = q+` WHERE domain = ?`, append(args, strings.ToLower(domain))
	}
	return db.queryMailboxes(ctx, q+` ORDER BY address`, args...)
}

// ListMailboxesLinkedTo returns the mailboxes linked to a user in one team.
func (db *DB) ListMailboxesLinkedTo(ctx context.Context, teamID, userID string) ([]Mailbox, error) {
	return db.queryMailboxes(ctx, `SELECT `+mailboxCols+` FROM mail_mailboxes WHERE team_id = ? AND linked_user_id = ? ORDER BY address`, teamID, userID)
}

func (db *DB) queryMailboxes(ctx context.Context, q string, args ...any) ([]Mailbox, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list mailboxes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Mailbox{}
	for rows.Next() {
		m, err := scanMailbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMailboxSuspended flips the suspended flag.
func (db *DB) SetMailboxSuspended(ctx context.Context, address string, suspended bool) error {
	res, err := db.ExecContext(ctx, `UPDATE mail_mailboxes SET suspended = ?, updated_at = ? WHERE address = ?`, boolInt(suspended), mailNow(), strings.ToLower(address))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMailboxNotFound
	}
	return nil
}

// DeleteMailbox removes the record.
func (db *DB) DeleteMailbox(ctx context.Context, address string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM mail_mailboxes WHERE address = ?`, strings.ToLower(address))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMailboxNotFound
	}
	return nil
}
