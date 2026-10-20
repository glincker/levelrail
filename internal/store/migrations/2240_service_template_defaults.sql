-- What a template or compose file set on a service by default, kept so the dashboard can say why.
CREATE TABLE service_template_defaults (
    service_name    TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    template_id     TEXT NOT NULL DEFAULT '',
    strategy        TEXT NOT NULL DEFAULT '',
    strategy_reason TEXT NOT NULL DEFAULT '',
    memory_bytes    INTEGER NOT NULL DEFAULT 0,
    updated_at      TEXT NOT NULL
);
