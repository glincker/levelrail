-- Split testing: send a share of an app's visitors to another deploy, sticky
-- per visitor. One running test per app; request counts are kept per hour.
CREATE TABLE split_tests (
    id              TEXT PRIMARY KEY,
    app_name        TEXT NOT NULL,
    status          TEXT NOT NULL,
    variants        TEXT NOT NULL,
    cookie_name     TEXT NOT NULL,
    cookie_ttl_secs INTEGER NOT NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    ended_at        TEXT NOT NULL DEFAULT '',
    end_reason      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_split_tests_app ON split_tests (app_name, created_at);
CREATE UNIQUE INDEX idx_split_tests_running ON split_tests (app_name) WHERE status = 'running';

CREATE TABLE split_test_stats (
    test_id    TEXT NOT NULL,
    variant_id TEXT NOT NULL,
    bucket     TEXT NOT NULL,
    requests   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (test_id, variant_id, bucket)
);
