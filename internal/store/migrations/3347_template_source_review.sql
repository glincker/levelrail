-- Commit and content pinning for template sources: a changed entry that was approved before needs a new review.
ALTER TABLE template_sources ADD COLUMN commit_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE template_source_entries ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE template_source_entries ADD COLUMN review TEXT NOT NULL DEFAULT 'approved';
