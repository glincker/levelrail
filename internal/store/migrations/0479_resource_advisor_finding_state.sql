-- Operator decisions on resource advisor findings, keyed by the finding's
-- stable id. A dismissed row hides the finding until the condition clears
-- and returns; a snoozed row hides it until snoozed_until passes.
CREATE TABLE resource_advisor_finding_state (
    finding_id    TEXT PRIMARY KEY,
    state         TEXT NOT NULL CHECK (state IN ('dismissed', 'snoozed')),
    snoozed_until TEXT NOT NULL DEFAULT '',
    actor         TEXT NOT NULL DEFAULT '',
    updated_at    TEXT NOT NULL
);
