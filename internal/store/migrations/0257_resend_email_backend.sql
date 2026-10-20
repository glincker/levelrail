-- Adds Resend as a third email backend alongside SMTP and SES
-- (0032_email_settings.sql). resend_from is the only new structural
-- column; the API key itself goes through internal/secrets like every
-- other email credential, keyed by store.EmailSettingsSecretsKey().
-- No ALTER ... DROP CONSTRAINT in SQLite, so this is the same
-- recreate-and-copy pattern 0054_oidc_provider.sql uses.

CREATE TABLE email_settings_new (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    backend           TEXT NOT NULL DEFAULT '' CHECK (backend IN ('', 'smtp', 'ses', 'resend')),
    smtp_host         TEXT NOT NULL DEFAULT '',
    smtp_port         INTEGER NOT NULL DEFAULT 0,
    smtp_username     TEXT NOT NULL DEFAULT '',
    smtp_from         TEXT NOT NULL DEFAULT '',
    ses_region        TEXT NOT NULL DEFAULT '',
    ses_access_key_id TEXT NOT NULL DEFAULT '',
    ses_from          TEXT NOT NULL DEFAULT '',
    resend_from       TEXT NOT NULL DEFAULT ''
);

INSERT INTO email_settings_new (id, backend, smtp_host, smtp_port, smtp_username, smtp_from, ses_region, ses_access_key_id, ses_from)
SELECT id, backend, smtp_host, smtp_port, smtp_username, smtp_from, ses_region, ses_access_key_id, ses_from FROM email_settings;

DROP TABLE email_settings;

ALTER TABLE email_settings_new RENAME TO email_settings;
