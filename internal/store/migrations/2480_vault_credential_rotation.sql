-- Vault credential rotation: when it was last rotated and when the kept previous credential expires. Timestamps only, no secrets.
ALTER TABLE vault_settings ADD COLUMN credential_rotated_at TEXT NOT NULL DEFAULT '';
ALTER TABLE vault_settings ADD COLUMN credential_previous_expires_at TEXT NOT NULL DEFAULT '';
