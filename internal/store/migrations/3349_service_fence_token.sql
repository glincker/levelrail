-- fence_token only ever grows: it is bumped in the same statement that moves a service to another node,
-- so a delayed destructive action from before the move carries a lower token and is refused.
ALTER TABLE desired_services ADD COLUMN fence_token INTEGER NOT NULL DEFAULT 1;
