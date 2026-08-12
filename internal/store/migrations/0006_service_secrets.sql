-- TASKS.md 1.7's deferred follow-up: env injection at container-create
-- time. CLAUDE.md 4.10 specifies per-app data encryption keys wrapped by
-- a master key held only by the control plane; these two tables hold
-- exactly that, and nothing else. Neither table nor this migration knows
-- how to encrypt or decrypt anything, that stays internal/secrets'
-- job: this is storage for opaque bytes, the same separation
-- reconcile_status keeps between "what happened" and "what decides".
--
-- service_secrets holds one wrapped DEK per service, generated the
-- first time a secret value is set for that service and reused for
-- every value after (CLAUDE.md 4.10: "wrap the DEK once, encrypt many
-- values fast with it").
CREATE TABLE service_secrets (
    service_name TEXT PRIMARY KEY,
    wrapped_dek BLOB NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- service_secret_values holds one AES-256-GCM ciphertext per
-- (service, env var name), produced by internal/secrets.EncryptValue
-- under that service's DEK above. The plaintext never reaches this
-- table or any other: it exists only in memory, decrypted immediately
-- before internal/reconcile/application hands it to
-- docker.Runtime.Create.
CREATE TABLE service_secret_values (
    service_name TEXT NOT NULL REFERENCES service_secrets (service_name),
    env_key TEXT NOT NULL,
    ciphertext BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (service_name, env_key)
);

-- desired_services needs to know WHICH env var names are secret-backed
-- without holding their values: GET /api/v1/apps (internal/api) returns
-- this table's env column directly, so a resolvable value sitting in it
-- would defeat the entire point of encrypting it elsewhere. This column
-- carries names only, the same shape spec.Service.Env's { secret: true }
-- keys already have in app.yaml, which is not itself a secret.
ALTER TABLE desired_services ADD COLUMN secret_env TEXT NOT NULL DEFAULT '[]';
