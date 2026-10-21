-- Sender identity, delivery caps, TLS options, SES over SMTP and the key-based
-- API providers. Recreate-and-copy (as 0257 does) because the backend CHECK
-- cannot be altered in place; credentials stay in internal/secrets.
CREATE TABLE email_settings_new (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    backend           TEXT NOT NULL DEFAULT '' CHECK (backend IN ('', 'smtp', 'ses', 'ses_smtp', 'resend', 'postmark', 'sendgrid', 'mailgun')),
    smtp_host         TEXT NOT NULL DEFAULT '',
    smtp_port         INTEGER NOT NULL DEFAULT 0,
    smtp_username     TEXT NOT NULL DEFAULT '',
    smtp_from         TEXT NOT NULL DEFAULT '',
    ses_region        TEXT NOT NULL DEFAULT '',
    ses_access_key_id TEXT NOT NULL DEFAULT '',
    ses_from          TEXT NOT NULL DEFAULT '',
    resend_from       TEXT NOT NULL DEFAULT '',
    from_name         TEXT NOT NULL DEFAULT '',
    from_address      TEXT NOT NULL DEFAULT '',
    reply_to          TEXT NOT NULL DEFAULT '',
    envelope_from     TEXT NOT NULL DEFAULT '',
    sends_per_minute  INTEGER NOT NULL DEFAULT 0,
    sends_per_day     INTEGER NOT NULL DEFAULT 0,
    tls_mode          TEXT NOT NULL DEFAULT '' CHECK (tls_mode IN ('', 'starttls', 'tls', 'none')),
    tls_skip_verify   INTEGER NOT NULL DEFAULT 0,
    ses_smtp_port     INTEGER NOT NULL DEFAULT 587,
    ses_smtp_username TEXT NOT NULL DEFAULT '',
    mailgun_domain    TEXT NOT NULL DEFAULT '',
    mailgun_region    TEXT NOT NULL DEFAULT 'us'
);

INSERT INTO email_settings_new (id, backend, smtp_host, smtp_port, smtp_username, smtp_from, ses_region, ses_access_key_id, ses_from, resend_from)
SELECT id, backend, smtp_host, smtp_port, smtp_username, smtp_from, ses_region, ses_access_key_id, ses_from, resend_from FROM email_settings;

DROP TABLE email_settings; -- compat-break: table rebuilt under the same name to add columns

ALTER TABLE email_settings_new RENAME TO email_settings;
