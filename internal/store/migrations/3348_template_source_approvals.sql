-- Approvals are kept per (source, id) so a returning id with the same content hash stays approved
-- and any other new id needs review. Rows synced before review existed become unreviewed.
CREATE TABLE IF NOT EXISTS template_source_approvals (
    source       TEXT NOT NULL,
    id           TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    PRIMARY KEY (source, id)
);
-- The column default stays 'approved' because rebuilding the table would break upgrades; every
-- insert sets review explicitly instead.
UPDATE template_source_entries SET review = 'needs_review';
