-- Instance id of the database a row was written for; rows from before this column keep an empty id.
ALTER TABLE database_statement_audit ADD COLUMN database_id TEXT NOT NULL DEFAULT '';
