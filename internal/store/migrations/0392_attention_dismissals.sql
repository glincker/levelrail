CREATE TABLE attention_dismissals (
    user_id      TEXT NOT NULL,
    item_key     TEXT NOT NULL,
    dismissed_at TEXT NOT NULL,
    PRIMARY KEY (user_id, item_key)
);

CREATE INDEX idx_attention_dismissals_at ON attention_dismissals (dismissed_at);
