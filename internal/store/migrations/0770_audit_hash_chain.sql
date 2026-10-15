-- Tamper evidence: each new audit_log row gets a seq, the previous row's
-- hash and its own hash. Rows written before this migration keep seq 0
-- (unchained) and are never rewritten.
ALTER TABLE audit_log ADD COLUMN seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audit_log ADD COLUMN prev_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN entry_hash TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_audit_log_seq ON audit_log (seq) WHERE seq > 0;

CREATE TABLE audit_chain_head (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    seq  INTEGER NOT NULL,
    hash TEXT NOT NULL
);
INSERT INTO audit_chain_head (id, seq, hash) VALUES (1, 0, '');
