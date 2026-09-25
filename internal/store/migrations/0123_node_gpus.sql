-- Latest NVIDIA GPU snapshot per node, written by the agent's GPU report
-- (remote nodes) or the control plane's own detection (local node).
-- Devices is a JSON array of {index, uuid, name, vram_total_mib,
-- vram_used_mib, utilization_percent}.
CREATE TABLE node_gpus (
    node_id           TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    present           INTEGER NOT NULL DEFAULT 0,
    driver_version    TEXT NOT NULL DEFAULT '',
    runtime_installed INTEGER NOT NULL DEFAULT 0,
    devices           TEXT NOT NULL DEFAULT '[]',
    updated_at        TEXT NOT NULL
);
