-- Cloud registry presets (ECR, Artifact Registry, ACR) mint short-lived passwords; preset_config holds non-secret settings only.
ALTER TABLE registry_credentials ADD COLUMN preset TEXT NOT NULL DEFAULT '';
ALTER TABLE registry_credentials ADD COLUMN preset_config TEXT NOT NULL DEFAULT '';
ALTER TABLE registry_credentials ADD COLUMN last_refreshed_at TEXT;
ALTER TABLE registry_credentials ADD COLUMN next_refresh_at TEXT;
ALTER TABLE registry_credentials ADD COLUMN last_refresh_error TEXT NOT NULL DEFAULT '';
