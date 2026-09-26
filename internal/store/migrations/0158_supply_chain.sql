-- Supply chain metadata per deploy attempt. SBOM documents live on disk under
-- DataDir/supplychain, never in this database.
CREATE TABLE IF NOT EXISTS deploy_supply_chain (
    attempt_id      TEXT PRIMARY KEY,
    app_name        TEXT NOT NULL,
    sbom_format     TEXT NOT NULL DEFAULT '',
    package_count   INTEGER NOT NULL DEFAULT 0,
    sbom_bytes      INTEGER NOT NULL DEFAULT 0,
    has_provenance  INTEGER NOT NULL DEFAULT 0,
    summary         TEXT NOT NULL DEFAULT '',
    generated_at    TEXT NOT NULL,
    scan_status     TEXT NOT NULL DEFAULT '',
    scanner         TEXT NOT NULL DEFAULT '',
    scan_error      TEXT NOT NULL DEFAULT '',
    scanned_at      TEXT NOT NULL DEFAULT '',
    vuln_counts     TEXT NOT NULL DEFAULT '',
    top_vulns       TEXT NOT NULL DEFAULT '',
    gate_action     TEXT NOT NULL DEFAULT '',
    gate_reason     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_deploy_supply_chain_app ON deploy_supply_chain (app_name, generated_at DESC);

CREATE TABLE IF NOT EXISTS app_supply_chain_settings (
    app_name            TEXT PRIMARY KEY,
    scan_enabled        INTEGER NOT NULL DEFAULT 0,
    scan_gate           TEXT NOT NULL DEFAULT 'off',
    override_reason     TEXT NOT NULL DEFAULT '',
    override_armed_at   TEXT NOT NULL DEFAULT '',
    updated_at          TEXT NOT NULL
);
