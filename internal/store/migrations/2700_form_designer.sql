-- Form designer: per-form schema JSON (empty object keeps the default fields) and a test-submission flag.
ALTER TABLE forms ADD COLUMN schema TEXT NOT NULL DEFAULT '{}';
ALTER TABLE form_submissions ADD COLUMN test INTEGER NOT NULL DEFAULT 0;
