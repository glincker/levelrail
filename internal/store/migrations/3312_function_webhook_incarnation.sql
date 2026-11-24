-- Binds a function webhook to one life of its app, so a signing secret outlives neither a delete nor a same-name create.
ALTER TABLE function_webhooks ADD COLUMN incarnation TEXT NOT NULL DEFAULT '';
