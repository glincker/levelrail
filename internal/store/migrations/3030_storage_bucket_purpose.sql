-- Same table backs app buckets and asset buckets (ADR 044); existing rows stay 'app'.
ALTER TABLE storage_buckets ADD COLUMN purpose TEXT NOT NULL DEFAULT 'app' CHECK (purpose IN ('app', 'assets'));
