-- Binds a function webhook to one life of its app, so a signing secret outlives neither a delete nor a same-name create.
ALTER TABLE function_webhooks ADD COLUMN incarnation TEXT NOT NULL DEFAULT '';

-- Rows left by apps deleted before the cleanup existed would let a stale secret or old data reach a same-name app.
DELETE FROM function_webhooks WHERE function NOT IN (SELECT name FROM desired_services)
    AND function NOT IN (SELECT name FROM recently_deleted WHERE kind = 'app');
DELETE FROM function_kv WHERE namespace NOT IN (SELECT name FROM desired_services)
    AND namespace NOT IN (SELECT name FROM recently_deleted WHERE kind = 'app');
DELETE FROM function_kv_tokens WHERE namespace NOT IN (SELECT name FROM desired_services)
    AND namespace NOT IN (SELECT name FROM recently_deleted WHERE kind = 'app');
-- KV data of a name with no token row and no app, live or in the undo window, belongs to no one.
DELETE FROM function_kv WHERE namespace NOT IN (SELECT namespace FROM function_kv_tokens)
    AND namespace NOT IN (SELECT name FROM desired_services)
    AND namespace NOT IN (SELECT name FROM recently_deleted WHERE kind = 'app');
