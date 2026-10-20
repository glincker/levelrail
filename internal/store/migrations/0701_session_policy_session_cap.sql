-- Per-team cap on concurrent full sessions per user, enforced by the API
-- middleware (the library cap is one global value, so it cannot serve teams).
ALTER TABLE session_policies ADD COLUMN max_concurrent_sessions INTEGER NOT NULL DEFAULT 0;
ALTER TABLE session_policies ADD COLUMN on_limit TEXT NOT NULL DEFAULT 'evict_oldest' CHECK (on_limit IN ('evict_oldest', 'reject'));
