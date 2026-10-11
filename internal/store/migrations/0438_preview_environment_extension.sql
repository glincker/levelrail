-- An operator can extend one preview past its TTL; the sweep honours this
-- instant (RFC3339Nano UTC, empty means not extended).
ALTER TABLE preview_environments ADD COLUMN extended_until TEXT NOT NULL DEFAULT '';
