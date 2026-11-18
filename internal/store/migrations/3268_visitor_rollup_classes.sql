-- Per-class request counts and page-only top paths for visitor rollups. Rows
-- written before this migration carry zeros and an empty list, since the class
-- of an old request was never recorded.
ALTER TABLE visitor_rollups ADD COLUMN n_other    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE visitor_rollups ADD COLUMN n_page     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE visitor_rollups ADD COLUMN n_api      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE visitor_rollups ADD COLUMN n_asset    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE visitor_rollups ADD COLUMN n_bot      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE visitor_rollups ADD COLUMN n_prefetch INTEGER NOT NULL DEFAULT 0;
ALTER TABLE visitor_rollups ADD COLUMN top_pages  TEXT    NOT NULL DEFAULT '[]';
