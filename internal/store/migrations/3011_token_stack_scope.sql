-- Stack scope for API tokens: a token bound to one stack may only plan or
-- apply that stack (ADR 042). Kept apart from api_token_scopes, which needs a team.
CREATE TABLE IF NOT EXISTS api_token_stack_scopes (
    token_id TEXT PRIMARY KEY,
    stack_id TEXT NOT NULL,
    verbs    TEXT NOT NULL DEFAULT 'plan'
);
