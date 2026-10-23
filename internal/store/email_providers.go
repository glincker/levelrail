package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Roles an email provider can hold. At most one provider per role exists.
const (
	EmailRolePrimary = "primary"
	EmailRoleBackup  = "backup"
)

// EmailPrimaryProviderID is the id the migration gives a backfilled primary.
const EmailPrimaryProviderID = "prov_primary"

// ErrEmailProviderInvalid wraps every validation failure of UpsertEmailProvider and DeleteEmailProvider.
var ErrEmailProviderInvalid = errors.New("store: invalid email provider")

// ErrEmailProviderNotFound is returned when no provider matches.
var ErrEmailProviderNotFound = errors.New("store: email provider not found")

// EmailProvider is one configured sending provider. No credential fields:
// those live in internal/secrets. Sender identity and caps stay in EmailSettings.
type EmailProvider struct {
	ID      string
	Role    string
	Name    string
	Enabled bool
	Backend string

	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPFrom        string
	SESRegion       string
	SESAccessKeyID  string
	SESFrom         string
	ResendFrom      string
	TLSMode         string
	TLSSkipVerify   bool
	SESSMTPPort     int
	SESSMTPUsername string
	MailgunDomain   string
	MailgunRegion   string

	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastOKAt      time.Time
	LastErrorKind string
}

const emailProviderColumns = `id, role, name, enabled, backend, smtp_host, smtp_port, smtp_username, smtp_from,
		ses_region, ses_access_key_id, ses_from, resend_from, tls_mode, tls_skip_verify,
		ses_smtp_port, ses_smtp_username, mailgun_domain, mailgun_region,
		created_at, updated_at, last_ok_at, last_error_kind`

type emailProviderScanner interface{ Scan(dest ...any) error }

func scanEmailProvider(r emailProviderScanner) (EmailProvider, error) {
	var p EmailProvider
	var enabled, skip int
	var created, updated, lastOK string
	err := r.Scan(&p.ID, &p.Role, &p.Name, &enabled, &p.Backend, &p.SMTPHost, &p.SMTPPort, &p.SMTPUsername, &p.SMTPFrom,
		&p.SESRegion, &p.SESAccessKeyID, &p.SESFrom, &p.ResendFrom, &p.TLSMode, &skip,
		&p.SESSMTPPort, &p.SESSMTPUsername, &p.MailgunDomain, &p.MailgunRegion,
		&created, &updated, &lastOK, &p.LastErrorKind)
	if err != nil {
		return EmailProvider{}, err
	}
	p.Enabled = enabled != 0
	p.TLSSkipVerify = skip != 0
	p.CreatedAt = parseProviderTime(created)
	p.UpdatedAt = parseProviderTime(updated)
	p.LastOKAt = parseProviderTime(lastOK)
	return p, nil
}

func parseProviderTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatProviderTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func validEmailBackend(b string) bool {
	switch b {
	case EmailBackendSMTP, EmailBackendSES, EmailBackendSESSMTP, EmailBackendResend,
		EmailBackendPostmark, EmailBackendSendGrid, EmailBackendMailgun:
		return true
	}
	return false
}

func invalidProvider(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrEmailProviderInvalid, fmt.Sprintf(format, args...))
}

// ListEmailProviders returns the configured providers, primary first.
func (db *DB) ListEmailProviders(ctx context.Context) ([]EmailProvider, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+emailProviderColumns+` FROM email_providers ORDER BY CASE role WHEN 'primary' THEN 0 ELSE 1 END`)
	if err != nil {
		return nil, fmt.Errorf("store: list email providers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EmailProvider
	for rows.Next() {
		p, err := scanEmailProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan email provider: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list email providers: %w", err)
	}
	return out, nil
}

// GetEmailProvider returns one provider by id, or ErrEmailProviderNotFound.
func (db *DB) GetEmailProvider(ctx context.Context, id string) (EmailProvider, error) {
	p, err := scanEmailProvider(db.QueryRowContext(ctx, `SELECT `+emailProviderColumns+` FROM email_providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return EmailProvider{}, ErrEmailProviderNotFound
	}
	if err != nil {
		return EmailProvider{}, fmt.Errorf("store: get email provider: %w", err)
	}
	return p, nil
}

// UpsertEmailProvider inserts or replaces a provider in full. A provider
// never changes role, and a backup needs an existing primary.
func (db *DB) UpsertEmailProvider(ctx context.Context, p EmailProvider) error {
	if p.ID == "" {
		return invalidProvider("id is required")
	}
	if p.Role != EmailRolePrimary && p.Role != EmailRoleBackup {
		return invalidProvider("role %q must be primary or backup", p.Role)
	}
	if !validEmailBackend(p.Backend) {
		return invalidProvider("backend %q is not supported", p.Backend)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: upsert email provider: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var holder string
	err = tx.QueryRowContext(ctx, `SELECT id FROM email_providers WHERE role = ?`, p.Role).Scan(&holder)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: upsert email provider: %w", err)
	}
	if err == nil && holder != p.ID {
		return invalidProvider("role %q is already held by %q", p.Role, holder)
	}
	var existingRole string
	err = tx.QueryRowContext(ctx, `SELECT role FROM email_providers WHERE id = ?`, p.ID).Scan(&existingRole)
	isNew := errors.Is(err, sql.ErrNoRows)
	if err != nil && !isNew {
		return fmt.Errorf("store: upsert email provider: %w", err)
	}
	if !isNew && existingRole != p.Role {
		return invalidProvider("provider %q cannot change role from %q to %q", p.ID, existingRole, p.Role)
	}
	if p.Role == EmailRoleBackup {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_providers WHERE role = 'primary'`).Scan(&n); err != nil {
			return fmt.Errorf("store: upsert email provider: %w", err)
		}
		if n == 0 {
			return invalidProvider("a backup requires an existing primary")
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	created := now
	if !p.CreatedAt.IsZero() {
		created = formatProviderTime(p.CreatedAt)
	}
	enabled, skip := 0, 0
	if p.Enabled {
		enabled = 1
	}
	if p.TLSSkipVerify {
		skip = 1
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO email_providers (`+emailProviderColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, enabled = excluded.enabled, backend = excluded.backend,
			smtp_host = excluded.smtp_host, smtp_port = excluded.smtp_port,
			smtp_username = excluded.smtp_username, smtp_from = excluded.smtp_from,
			ses_region = excluded.ses_region, ses_access_key_id = excluded.ses_access_key_id,
			ses_from = excluded.ses_from, resend_from = excluded.resend_from,
			tls_mode = excluded.tls_mode, tls_skip_verify = excluded.tls_skip_verify,
			ses_smtp_port = excluded.ses_smtp_port, ses_smtp_username = excluded.ses_smtp_username,
			mailgun_domain = excluded.mailgun_domain, mailgun_region = excluded.mailgun_region,
			updated_at = excluded.updated_at, last_ok_at = excluded.last_ok_at,
			last_error_kind = excluded.last_error_kind
	`, p.ID, p.Role, p.Name, enabled, p.Backend, p.SMTPHost, p.SMTPPort, p.SMTPUsername, p.SMTPFrom,
		p.SESRegion, p.SESAccessKeyID, p.SESFrom, p.ResendFrom, p.TLSMode, skip,
		p.SESSMTPPort, p.SESSMTPUsername, p.MailgunDomain, p.MailgunRegion,
		created, now, formatProviderTime(p.LastOKAt), p.LastErrorKind)
	if err != nil {
		return fmt.Errorf("store: upsert email provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: upsert email provider: %w", err)
	}
	return nil
}

// DeleteEmailProvider removes a provider. The primary cannot be deleted
// while a backup exists. Unknown ids return ErrEmailProviderNotFound.
func (db *DB) DeleteEmailProvider(ctx context.Context, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete email provider: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var role string
	err = tx.QueryRowContext(ctx, `SELECT role FROM email_providers WHERE id = ?`, id).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEmailProviderNotFound
	}
	if err != nil {
		return fmt.Errorf("store: delete email provider: %w", err)
	}
	if role == EmailRolePrimary {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_providers WHERE role = 'backup'`).Scan(&n); err != nil {
			return fmt.Errorf("store: delete email provider: %w", err)
		}
		if n > 0 {
			return invalidProvider("the primary cannot be deleted while a backup exists")
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_providers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete email provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete email provider: %w", err)
	}
	return nil
}
