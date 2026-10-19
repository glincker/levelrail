ALTER TABLE audit_log ADD COLUMN chain_seq INTEGER;
ALTER TABLE audit_log ADD COLUMN chain_prev TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN chain_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN chain_alg TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_audit_log_chain_seq ON audit_log (chain_seq) WHERE chain_seq IS NOT NULL;

CREATE TABLE audit_chain_links (
    seq_from INTEGER PRIMARY KEY,
    seq_to   INTEGER NOT NULL,
    prev     TEXT NOT NULL,
    hash     TEXT NOT NULL
);

CREATE TABLE audit_chain_checkpoints (
    seq        INTEGER PRIMARY KEY,
    hash       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    sig        TEXT NOT NULL
);
