-- Platform-wide email-sending configuration: which backend (SMTP or AWS
-- SES) internal/email.Sender uses for both internal/alerting's
-- alert-rule/deploy-outcome email notifications and password-reset
-- emails. One authoritative row (id = 1), the same singleton-settings
-- shape ingress_settings (migrations/0024) already establishes.
--
-- No smtp_password or ses_secret_access_key column here: those go
-- through internal/secrets' envelope encryption, keyed by
-- store.EmailSettingsSecretsKey(), the same "credential goes through
-- internal/secrets, never as a plaintext column" reasoning
-- migrations/0018_backup_targets.sql already establishes for a backup
-- target's own credentials.
--
-- backend is '' (unset) by default: a fresh install with no operator
-- configuration falls back to APP_SMTP_* env vars if those are set (see
-- cmd/levelrail/main.go), matching this table's upgrade-safe default of
-- "byte-identical to pre-migration behavior" that ingress_settings'
-- acme_enabled = 0 default already establishes for its own table.
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
