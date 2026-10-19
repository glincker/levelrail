ALTER TABLE instance_identity ADD COLUMN fingerprint_key TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_identity ADD COLUMN host_marker TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_identity ADD COLUMN previous_instance_id TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_identity ADD COLUMN previous_fingerprint_key TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_identity ADD COLUMN moved_at TEXT NOT NULL DEFAULT '';

CREATE TABLE api_token_bindings (
    token_id     TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL,
    bound_at     TEXT NOT NULL,
    last_ip      TEXT NOT NULL DEFAULT '',
    last_seen_at TEXT NOT NULL DEFAULT ''
);
