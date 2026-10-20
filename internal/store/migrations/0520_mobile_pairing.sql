-- One-time codes a signed-in admin shows as a QR so a phone can pair.
-- Only the SHA-256 of the full code is stored; lookup is the public prefix.
CREATE TABLE mobile_pairing_codes (
    id            TEXT PRIMARY KEY,
    lookup        TEXT NOT NULL UNIQUE,
    code_hash     TEXT NOT NULL,
    owner_user_id TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    used_at       TEXT,
    failed_tries  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_mobile_pairing_codes_expires ON mobile_pairing_codes (expires_at);

-- A paired phone. The credential itself is an ordinary API token row.
CREATE TABLE mobile_devices (
    id            TEXT PRIMARY KEY,
    token_id      TEXT NOT NULL UNIQUE,
    owner_user_id TEXT NOT NULL,
    name          TEXT NOT NULL,
    platform      TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    revoked_at    TEXT
);

CREATE INDEX idx_mobile_devices_owner ON mobile_devices (owner_user_id);
