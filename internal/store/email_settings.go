package store

import (
	"context"
	"fmt"
)

// Email backends store.EmailSettings.Backend accepts. Empty string means
// "unset": the caller falls back to APP_SMTP_* env vars if set.
const (
	EmailBackendSMTP     = "smtp"
	EmailBackendSES      = "ses"
	EmailBackendSESSMTP  = "ses_smtp"
	EmailBackendResend   = "resend"
	EmailBackendPostmark = "postmark"
	EmailBackendSendGrid = "sendgrid"
	EmailBackendMailgun  = "mailgun"
)

// EmailSettingsSecretsKey is the internal/secrets serviceName the
// platform-wide email settings' credentials are stored under.
func EmailSettingsSecretsKey() string {
	return "email-settings"
}

// The internal/secrets envKeys email credentials are stored under, shared by the API and the send path.
const (
	EmailSecretSMTPPassword    = "smtp_password"         //nolint:gosec // an envKey name, not a credential value
	EmailSecretSESSecretKey    = "ses_secret_access_key" //nolint:gosec // an envKey name, not a credential value
	EmailSecretSESSMTPPassword = "ses_smtp_password"     //nolint:gosec // an envKey name, not a credential value
	EmailSecretResendAPIKey    = "resend_api_key"        //nolint:gosec // an envKey name, not a credential value
	EmailSecretPostmarkToken   = "postmark_server_token" //nolint:gosec // an envKey name, not a credential value
	EmailSecretSendGridAPIKey  = "sendgrid_api_key"      //nolint:gosec // an envKey name, not a credential value
	EmailSecretMailgunAPIKey   = "mailgun_api_key"       //nolint:gosec // an envKey name, not a credential value
)

// EmailSettings is the single platform-wide email-sending configuration
// row. No credential fields: those go through internal/secrets instead.
type EmailSettings struct {
	Backend        string
	SMTPHost       string
	SMTPPort       int
	SMTPUsername   string
	SMTPFrom       string
	SESRegion      string
	SESAccessKeyID string
	SESFrom        string
	ResendFrom     string

	// Sender identity shared by every provider; FromAddress wins over the legacy per-provider from columns.
	FromName     string
	FromAddress  string
	ReplyTo      string
	EnvelopeFrom string

	// Zero means "use the APP_EMAIL_SENDS_PER_* env default" (itself unlimited when unset).
	SendsPerMinute int
	SendsPerDay    int

	// TLSMode is "", "starttls", "tls" or "none"; TLSSkipVerify turns certificate verification off.
	TLSMode       string
	TLSSkipVerify bool

	SESSMTPPort     int
	SESSMTPUsername string
	MailgunDomain   string
	MailgunRegion   string
}

// EffectiveFrom is the sender address the active backend will use.
func (s EmailSettings) EffectiveFrom() string {
	if s.FromAddress != "" {
		return s.FromAddress
	}
	switch s.Backend {
	case EmailBackendSMTP:
		return s.SMTPFrom
	case EmailBackendSES, EmailBackendSESSMTP:
		return s.SESFrom
	case EmailBackendResend:
		return s.ResendFrom
	}
	return ""
}

const emailSettingsColumns = `backend, smtp_host, smtp_port, smtp_username, smtp_from,
		ses_region, ses_access_key_id, ses_from, resend_from,
		from_name, from_address, reply_to, envelope_from,
		sends_per_minute, sends_per_day, tls_mode, tls_skip_verify,
		ses_smtp_port, ses_smtp_username, mailgun_domain, mailgun_region`

// GetEmailSettings returns the single email_settings row. Always
// succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetEmailSettings(ctx context.Context) (EmailSettings, error) {
	var s EmailSettings
	var skip int
	err := db.QueryRowContext(ctx, `SELECT `+emailSettingsColumns+` FROM email_settings WHERE id = 1`).Scan(
		&s.Backend, &s.SMTPHost, &s.SMTPPort, &s.SMTPUsername, &s.SMTPFrom,
		&s.SESRegion, &s.SESAccessKeyID, &s.SESFrom, &s.ResendFrom,
		&s.FromName, &s.FromAddress, &s.ReplyTo, &s.EnvelopeFrom,
		&s.SendsPerMinute, &s.SendsPerDay, &s.TLSMode, &skip,
		&s.SESSMTPPort, &s.SESSMTPUsername, &s.MailgunDomain, &s.MailgunRegion)
	if err != nil {
		return EmailSettings{}, fmt.Errorf("store: get email settings: %w", err)
	}
	s.TLSSkipVerify = skip != 0
	return s, nil
}

// UpdateEmailSettings replaces the single email_settings row in full,
// the same whole-record convention UpdateIngressSettings uses.
func (db *DB) UpdateEmailSettings(ctx context.Context, s EmailSettings) error {
	skip := 0
	if s.TLSSkipVerify {
		skip = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE email_settings
		SET backend = ?, smtp_host = ?, smtp_port = ?, smtp_username = ?, smtp_from = ?,
		    ses_region = ?, ses_access_key_id = ?, ses_from = ?, resend_from = ?,
		    from_name = ?, from_address = ?, reply_to = ?, envelope_from = ?,
		    sends_per_minute = ?, sends_per_day = ?, tls_mode = ?, tls_skip_verify = ?,
		    ses_smtp_port = ?, ses_smtp_username = ?, mailgun_domain = ?, mailgun_region = ?
		WHERE id = 1
	`, s.Backend, s.SMTPHost, s.SMTPPort, s.SMTPUsername, s.SMTPFrom,
		s.SESRegion, s.SESAccessKeyID, s.SESFrom, s.ResendFrom,
		s.FromName, s.FromAddress, s.ReplyTo, s.EnvelopeFrom,
		s.SendsPerMinute, s.SendsPerDay, s.TLSMode, skip,
		s.SESSMTPPort, s.SESSMTPUsername, s.MailgunDomain, s.MailgunRegion)
	if err != nil {
		return fmt.Errorf("store: update email settings: %w", err)
	}
	return nil
}
