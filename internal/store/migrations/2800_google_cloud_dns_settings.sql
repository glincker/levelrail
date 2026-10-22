-- Platform-wide Google Cloud DNS config, same single-seeded-row shape as
-- route53_dns_settings. project_id and client_email are read from the
-- service account key at save time and kept here only for display. The key
-- itself goes through internal/secrets under store.GoogleCloudDNSSecretsKey().
CREATE TABLE google_cloud_dns_settings (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    enabled      INTEGER NOT NULL DEFAULT 0,
    project_id   TEXT NOT NULL DEFAULT '',
    client_email TEXT NOT NULL DEFAULT ''
);

INSERT INTO google_cloud_dns_settings (id) VALUES (1);
