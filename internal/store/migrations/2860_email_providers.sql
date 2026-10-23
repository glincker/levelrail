-- One primary and at most one backup email provider. Holds only the non-secret,
-- per-provider fields; identity, caps and credentials stay where they are
-- (email_settings, internal/secrets), so email_settings is left untouched.
CREATE TABLE email_providers (
    id                TEXT PRIMARY KEY,
    role              TEXT NOT NULL CHECK (role IN ('primary', 'backup')),
    name              TEXT NOT NULL DEFAULT '',
    enabled           INTEGER NOT NULL DEFAULT 1,
    backend           TEXT NOT NULL CHECK (backend IN ('smtp', 'ses', 'ses_smtp', 'resend', 'postmark', 'sendgrid', 'mailgun')),
    smtp_host         TEXT NOT NULL DEFAULT '',
    smtp_port         INTEGER NOT NULL DEFAULT 0,
    smtp_username     TEXT NOT NULL DEFAULT '',
    smtp_from         TEXT NOT NULL DEFAULT '',
    ses_region        TEXT NOT NULL DEFAULT '',
    ses_access_key_id TEXT NOT NULL DEFAULT '',
    ses_from          TEXT NOT NULL DEFAULT '',
    resend_from       TEXT NOT NULL DEFAULT '',
    tls_mode          TEXT NOT NULL DEFAULT '' CHECK (tls_mode IN ('', 'starttls', 'tls', 'none')),
    tls_skip_verify   INTEGER NOT NULL DEFAULT 0,
    ses_smtp_port     INTEGER NOT NULL DEFAULT 587,
    ses_smtp_username TEXT NOT NULL DEFAULT '',
    mailgun_domain    TEXT NOT NULL DEFAULT '',
    mailgun_region    TEXT NOT NULL DEFAULT 'us',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    last_ok_at        TEXT NOT NULL DEFAULT '',
    last_error_kind   TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX email_providers_role ON email_providers (role);

-- An existing configured backend becomes the primary provider.
INSERT INTO email_providers (id, role, name, enabled, backend, smtp_host, smtp_port, smtp_username, smtp_from,
    ses_region, ses_access_key_id, ses_from, resend_from, tls_mode, tls_skip_verify,
    ses_smtp_port, ses_smtp_username, mailgun_domain, mailgun_region, created_at, updated_at)
SELECT 'prov_primary', 'primary', backend, 1, backend, smtp_host, smtp_port, smtp_username, smtp_from,
    ses_region, ses_access_key_id, ses_from, resend_from, tls_mode, tls_skip_verify,
    ses_smtp_port, ses_smtp_username, mailgun_domain, mailgun_region,
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM email_settings WHERE id = 1 AND backend <> '';
