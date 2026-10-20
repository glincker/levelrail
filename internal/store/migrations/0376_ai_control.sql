-- The master switch for agent and AI access, enforced server side. A fresh
-- instance starts off. An instance that already has live agent tokens keeps
-- working by starting in operate mode.
CREATE TABLE ai_control_settings (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    mode              TEXT NOT NULL DEFAULT 'off' CHECK (mode IN ('off', 'observe', 'operate', 'admin')),
    allowed_env_kinds TEXT NOT NULL DEFAULT '["dev","test","uat","preview"]',
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_by        TEXT NOT NULL DEFAULT ''
);

INSERT INTO ai_control_settings (id) VALUES (1);

UPDATE ai_control_settings SET mode = 'operate'
WHERE EXISTS (SELECT 1 FROM api_tokens WHERE agent_name != '' AND revoked_at IS NULL);
