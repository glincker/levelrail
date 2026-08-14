-- Phase 3 (TASKS.md 3.6): distributed cert storage. Backs
-- internal/ingress.SQLiteStorage, a certmagic.Storage implementation
-- that replaces Caddy's default file-system storage module
-- (internal/ingress.FileStorage) for certificates and ACME account
-- state, exactly the requirement internal/reconcile/ingress/controller.go's
-- own package doc comment already named (CLAUDE.md 4.5: "certificate
-- storage goes in the database so multi-node deployments share cert
-- state"). Every ingress-driving process pointed at this same database
-- file shares one view of every issued certificate, so two of them
-- never independently re-obtain a certificate for a domain the other
-- already has one for.
--
-- key/value/modified_at mirrors certmagic.Storage's own key-value
-- contract directly (see certmagic.Storage's doc comment: keys are
-- forward-slash-separated paths with directory semantics implied by
-- prefix, not modeled as literal parent rows here); internal/ingress is
-- responsible for the prefix/List/Stat semantics layered on top of this
-- flat table, this migration only needs exact-key lookup and prefix
-- scan (a LIKE on the key's leading bytes) to both be cheap, which the
-- PRIMARY KEY on key already gives for both cases.
CREATE TABLE cert_storage (
	key         TEXT PRIMARY KEY,
	value       BLOB NOT NULL,
	modified_at TEXT NOT NULL
);

-- Backs certmagic.Storage's embedded Locker: coordinates certificate
-- issuance so two ingress-driving processes sharing this database never
-- race to obtain the same certificate concurrently. A row older than
-- the staleness threshold internal/ingress checks in code is treated as
-- abandoned by a holder that crashed without calling Unlock, mirroring
-- certmagic's own FileStorage doc comment on stale-lock handling; there
-- is deliberately no staleness logic in SQL here, SQLite migrations are
-- schema only, per CLAUDE.md 4.7.
CREATE TABLE cert_storage_locks (
	name        TEXT PRIMARY KEY,
	acquired_at TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
