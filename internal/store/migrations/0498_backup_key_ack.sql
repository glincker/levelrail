-- The operator's statement that the backup encryption private key is stored
-- off this server. Tied to the public key so a rotated key asks again.
CREATE TABLE backup_key_ack (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    recipient       TEXT NOT NULL,
    acknowledged_at TEXT NOT NULL,
    acknowledged_by TEXT NOT NULL DEFAULT ''
);
