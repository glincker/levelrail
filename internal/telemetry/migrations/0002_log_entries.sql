-- One row per container log line. A normal rowid table, unlike
-- metric_samples' WITHOUT ROWID choice: log_entries_fts below needs a
-- stable integer key to correlate its index against (content_rowid), and
-- "id" doubling as SQLite's own rowid is the simplest way to get one.
--
-- resource_id is a plain string, not a foreign key, for the same reason
-- metric_samples.resource_id isn't one (see 0001): this store must keep
-- collecting and stay queryable for a resource whose desired state has
-- since been deleted.
--
-- structured/fields_json implement TASKS.md 2.2's "structured log
-- parsing where the app emits JSON": a line whose full text is valid
-- JSON is flagged structured = 1 and its parsed form is duplicated into
-- fields_json, so a future log viewer (TASKS.md 2.4) can query or render
-- it distinctly without re-parsing message on every read. A line that
-- doesn't parse as JSON, the common case for most containers, gets
-- structured = 0 and a NULL fields_json: not an error, just the normal
-- shape for plain-text output.
CREATE TABLE log_entries (
    id          INTEGER PRIMARY KEY,
    resource_id TEXT NOT NULL,
    stream      TEXT NOT NULL,
    ts          INTEGER NOT NULL, -- unix nanoseconds: log lines need sub-second ordering (many lines/sec from one container), unlike metric_samples' already-deduped 15s-tick seconds resolution
    message     TEXT NOT NULL,
    structured  INTEGER NOT NULL DEFAULT 0,
    fields_json TEXT
);

-- Supports both QueryLogs (resource_id + time range) and RetainLogs (a
-- pure timestamp sweep would still benefit from resource_id being first
-- in a composite index for the common query shape, the same reasoning
-- 0001's idx_metric_samples_ts note applies, adapted here to a composite
-- since log queries are always scoped to one resource_id).
CREATE INDEX idx_log_entries_resource_ts ON log_entries (resource_id, ts);

-- Full-text index over message only. modernc.org/sqlite v1.56.0 (this
-- project's pure-Go SQLite driver, ADR 007) is built with
-- SQLITE_ENABLE_FTS5, confirmed by a standalone runtime check (CREATE
-- VIRTUAL TABLE ... USING fts5, an INSERT, and a MATCH query all
-- succeeding) before committing to this design, not assumed from the
-- driver's documentation alone: pure-Go SQLite ports sometimes ship with
-- extensions compiled out, so this was worth verifying directly rather
-- than guessing.
--
-- content='log_entries' / content_rowid='id' makes this an external
-- content table: the FTS index stores only the token postings needed to
-- search, not a second copy of every message's full text, which is this
-- project's answer to the "compress" goal in the log store's design
-- (see logs.go's package doc comment for why an explicit
-- application-level compression step was rejected instead).
CREATE VIRTUAL TABLE log_entries_fts USING fts5(
    message,
    content='log_entries',
    content_rowid='id',
    tokenize='unicode61'
);

-- log_entries is append-only (WriteLogBatch only ever inserts, never
-- updates a log line), so only insert and delete triggers are needed to
-- keep the FTS index in sync; there's deliberately no update trigger for
-- a write pattern this table never performs.
CREATE TRIGGER log_entries_ai AFTER INSERT ON log_entries BEGIN
    INSERT INTO log_entries_fts(rowid, message) VALUES (new.id, new.message);
END;

-- Required for RetainLogs' sweep: without this, a deleted log_entries
-- row would leave an orphaned entry in the FTS index forever, since
-- SQLite doesn't maintain external-content FTS tables automatically.
CREATE TRIGGER log_entries_ad AFTER DELETE ON log_entries BEGIN
    INSERT INTO log_entries_fts(log_entries_fts, rowid, message) VALUES ('delete', old.id, old.message);
END;
