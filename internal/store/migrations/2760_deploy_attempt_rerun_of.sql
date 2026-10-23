-- Lineage for retries and rebuilds: the deploy attempt this one re-ran, NULL when it is not a rerun.
ALTER TABLE deploy_attempts ADD COLUMN rerun_of TEXT;
