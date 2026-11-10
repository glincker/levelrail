-- Permanent app protection: a protected app refuses deploys, scaling,
-- config writes and deletion until someone unprotects it, with the reason kept.
CREATE TABLE app_protection (
    app_name TEXT PRIMARY KEY,
    reason TEXT NOT NULL,
    protected_by TEXT NOT NULL DEFAULT '',
    protected_at TEXT NOT NULL
);
