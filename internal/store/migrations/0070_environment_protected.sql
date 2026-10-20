-- Marks an environment as protected: a deploy, rollback, or promotion
-- targeting an app tagged with it must set confirm: true (internal/api's
-- deploys.go and promote.go), the same "acknowledge before proceeding"
-- friction step a destructive delete already gets, not a second-actor
-- approval system.

ALTER TABLE environments ADD COLUMN protected INTEGER NOT NULL DEFAULT 0;
