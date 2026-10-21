-- Per-node host guardrail overrides and the opt-in auto-clean switch; NULL means use the env default.
CREATE TABLE node_guardrails (
    node_id             TEXT PRIMARY KEY,
    disk_warn_free_pct  REAL,
    disk_crit_free_pct  REAL,
    inode_warn_free_pct REAL,
    inode_crit_free_pct REAL,
    mem_warn_avail_pct  REAL,
    mem_crit_avail_pct  REAL,
    swap_warn_used_pct  REAL,
    swap_crit_used_pct  REAL,
    auto_clean          INTEGER NOT NULL DEFAULT 0,
    updated_at          TEXT NOT NULL,
    updated_by          TEXT NOT NULL DEFAULT ''
);
