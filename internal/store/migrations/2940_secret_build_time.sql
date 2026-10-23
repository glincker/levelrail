-- Default-deny opt-in: only a secret flagged build_time may be named in
-- app.yaml build.secrets. Existing secrets stay denied.
ALTER TABLE service_secret_values ADD COLUMN build_time INTEGER NOT NULL DEFAULT 0;
