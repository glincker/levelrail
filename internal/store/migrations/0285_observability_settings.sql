-- Platform-wide observability config: the external Grafana/dashboard URL
-- an operator points at wherever they already run Grafana, mirroring
-- cloudflare_tunnel_settings' (migrations/0049) single seeded row. Not a
-- secret: this is a plain URL, no envelope encryption needed.
CREATE TABLE observability_settings (
    id                   INTEGER PRIMARY KEY CHECK (id = 1),
    external_dashboard_url TEXT NOT NULL DEFAULT ''
);

INSERT INTO observability_settings (id) VALUES (1);
