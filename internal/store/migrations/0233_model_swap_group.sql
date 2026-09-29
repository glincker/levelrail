-- Model swap groups: models in the same non-empty swap_group on the same
-- node/GPU cannot both be resident; waking one stops the group's current
-- resident sibling first to free VRAM.
ALTER TABLE models ADD COLUMN swap_group TEXT NOT NULL DEFAULT '';
