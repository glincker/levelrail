-- Production certificate attempts per hostname, so the rate-limit budget survives a restart.
CREATE TABLE cert_ledger (
    domain TEXT NOT NULL,
    kind   TEXT NOT NULL,
    at     TEXT NOT NULL,
    PRIMARY KEY (domain, kind, at)
);
CREATE INDEX cert_ledger_at ON cert_ledger (at);
