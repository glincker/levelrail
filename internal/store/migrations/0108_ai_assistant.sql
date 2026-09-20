-- BYOK AI assistant chat: platform-wide provider/model config (the API
-- key itself goes through internal/secrets, keyed by
-- store.AIAssistantSecretsKey(), never a plaintext column here), plus
-- chat session/message/tool-confirmation history.
CREATE TABLE ai_assistant_settings (
    id       INTEGER PRIMARY KEY CHECK (id = 1),
    provider TEXT NOT NULL DEFAULT '',
    model    TEXT NOT NULL DEFAULT ''
);

INSERT INTO ai_assistant_settings (id) VALUES (1);

CREATE TABLE ai_chat_sessions (
    id         TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- role is 'user', 'assistant', or 'tool' (a tool_result turn). tool_use_id
-- is set only on a 'tool' role row, linking it back to the assistant
-- message's tool_calls entry it resolves; never exposed over the API,
-- purely so the Anthropic-format request can be reconstructed from
-- history. tool_calls is a JSON array (aiToolCallRecord), NULL when the
-- turn proposed none.
CREATE TABLE ai_chat_messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES ai_chat_sessions(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    content     TEXT NOT NULL DEFAULT '',
    tool_use_id TEXT NOT NULL DEFAULT '',
    tool_calls  TEXT,
    created_at  TIMESTAMP NOT NULL
);

CREATE INDEX idx_ai_chat_messages_session ON ai_chat_messages(session_id, created_at);

-- One row per mutating tool call the model proposed, pending a human
-- confirmation click (POST .../confirmations/{id}) before it ever
-- executes: the read/write boundary in internal/ai's own classifier is
-- enforced by never persisting a mutating call as anything but pending
-- until that click happens.
CREATE TABLE ai_chat_confirmations (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES ai_chat_sessions(id) ON DELETE CASCADE,
    message_id   TEXT NOT NULL REFERENCES ai_chat_messages(id) ON DELETE CASCADE,
    tool_use_id  TEXT NOT NULL,
    tool_name    TEXT NOT NULL,
    tool_input   TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    result       TEXT,
    created_at   TIMESTAMP NOT NULL,
    resolved_at  TIMESTAMP
);

CREATE INDEX idx_ai_chat_confirmations_message ON ai_chat_confirmations(message_id);
