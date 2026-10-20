-- Platform-wide email-sending config: which backend (SMTP or SES)
-- internal/email.Sender uses. No credential columns: smtp_password and
-- ses_secret_access_key go through internal/secrets instead, keyed by
-- store.EmailSettingsSecretsKey(). backend is '' by default so an
-- upgrade falls back to APP_SMTP_* env vars until an operator saves this.
CREATE TABLE email_settings (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    backend           TEXT NOT NULL DEFAULT '' CHECK (backend IN ('', 'smtp', 'ses')),
    smtp_host         TEXT NOT NULL DEFAULT '',
    smtp_port         INTEGER NOT NULL DEFAULT 0,
    smtp_username     TEXT NOT NULL DEFAULT '',
    smtp_from         TEXT NOT NULL DEFAULT '',
    ses_region        TEXT NOT NULL DEFAULT '',
    ses_access_key_id TEXT NOT NULL DEFAULT '',
    ses_from          TEXT NOT NULL DEFAULT ''
);

INSERT INTO email_settings (id) VALUES (1);
