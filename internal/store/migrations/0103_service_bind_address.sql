-- Which network interface an application service's published port
-- binds to (internal/bindaddr): "private" (loopback only), "public"
-- (every interface), or a literal IP. Every published port bound to
-- 0.0.0.0 unconditionally, with no way to restrict it, was a real
-- security gap (see internal/reconcile/application's toContainerSpec).
--
-- New rows default to 'private' at the Go layer (store.DefaultBindAddress,
-- service.go), closing that gap for anything created from here on.
-- Existing rows are backfilled to 'public' below, preserving the
-- exposure an operator already relied on until their next deploy
-- explicitly sets bind_address (or leaves it unset and picks up the new
-- 'private' default then): not silently dropping an existing service's
-- reachability the moment this migration runs.
ALTER TABLE desired_services ADD COLUMN bind_address TEXT NOT NULL DEFAULT 'private';
UPDATE desired_services SET bind_address = 'public';
