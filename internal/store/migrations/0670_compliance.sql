-- Access review campaigns, their per-subject rows, and evidence pack jobs.
CREATE TABLE access_review_campaigns (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'open',
    due_at       TEXT NOT NULL,
    repeat_days  INTEGER NOT NULL DEFAULT 0,
    due_days     INTEGER NOT NULL DEFAULT 14,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE access_review_items (
    id             TEXT PRIMARY KEY,
    campaign_id    TEXT NOT NULL REFERENCES access_review_campaigns(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    subject_id     TEXT NOT NULL,
    label          TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT '',
    detail         TEXT NOT NULL DEFAULT '',
    last_active_at TEXT NOT NULL DEFAULT '',
    decision       TEXT NOT NULL DEFAULT '',
    decided_by     TEXT NOT NULL DEFAULT '',
    decided_at     TEXT NOT NULL DEFAULT '',
    applied_at     TEXT NOT NULL DEFAULT '',
    apply_error    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_access_review_items_campaign ON access_review_items (campaign_id);

CREATE TABLE evidence_packs (
    id          TEXT PRIMARY KEY,
    range_from  TEXT NOT NULL,
    range_to    TEXT NOT NULL,
    status      TEXT NOT NULL,
    step        TEXT NOT NULL DEFAULT '',
    progress    INTEGER NOT NULL DEFAULT 0,
    file_name   TEXT NOT NULL DEFAULT '',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    sha256      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT ''
);
