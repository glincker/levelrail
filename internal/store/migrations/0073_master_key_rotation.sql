CREATE TABLE master_key_rotation (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    rotated_at TEXT NOT NULL
);
