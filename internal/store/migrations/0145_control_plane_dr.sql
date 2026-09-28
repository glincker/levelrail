-- Off-box disaster recovery for the control plane database: one settings row
-- plus the last backup, escrow and drill outcomes. No key material lives here,
-- only age PUBLIC recipients. A retain value of 0 means use the env default.
CREATE TABLE control_plane_dr_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    install_id TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 0,
    target_id TEXT NOT NULL DEFAULT '',
    recipients TEXT NOT NULL DEFAULT '[]',
    schedule TEXT NOT NULL DEFAULT '',
    retain_daily INTEGER NOT NULL DEFAULT 0,
    retain_weekly INTEGER NOT NULL DEFAULT 0,
    retain_monthly INTEGER NOT NULL DEFAULT 0,
    drill_schedule TEXT NOT NULL DEFAULT '',
    escrow_target_id TEXT NOT NULL DEFAULT '',
    escrow_generated_at TEXT NOT NULL DEFAULT '',
    escrow_acked_at TEXT NOT NULL DEFAULT '',
    last_backup_at TEXT NOT NULL DEFAULT '',
    last_backup_key TEXT NOT NULL DEFAULT '',
    last_backup_error TEXT NOT NULL DEFAULT '',
    last_attempt_at TEXT NOT NULL DEFAULT '',
    last_drill_at TEXT NOT NULL DEFAULT '',
    last_drill_ok INTEGER NOT NULL DEFAULT 0,
    last_drill_partial INTEGER NOT NULL DEFAULT 0,
    last_drill_detail TEXT NOT NULL DEFAULT '',
    last_drill_ms INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT ''
);
INSERT INTO control_plane_dr_settings (id) VALUES (1);
