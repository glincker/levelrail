-- SSH-based node provisioning: adopts a machine the operator already has
-- (any VPS, home server, Raspberry Pi) by installing the node agent over
-- SSH, tracked through to the join-token enrollment it's expected to
-- complete. Mirrors node_provisions (0210_node_provisions.sql), but for
-- an existing machine instead of one internal/provision creates at a
-- cloud provider: no provider/region/size/provider_server_id, and adds
-- detected_os/detected_arch (learned live over the SSH session, not
-- known up front the way a provider's own catalog names an image) and
-- log (the SSH install's own streamed progress, which a cloud-init
-- document gives no visibility into once handed to the provider).
-- Deliberately no host/username/credential columns: the SSH host and
-- credential are used once, in memory, for the provisioning goroutine's
-- lifetime, and never persisted anywhere.
CREATE TABLE IF NOT EXISTS ssh_node_provisions (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'general',
    status         TEXT NOT NULL DEFAULT 'connecting',
    detected_os    TEXT NOT NULL DEFAULT '',
    detected_arch  TEXT NOT NULL DEFAULT '',
    node_id        TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    log            TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ssh_node_provisions_status ON ssh_node_provisions (status);
