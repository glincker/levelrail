-- Branding every rich email wears, kept on the sender identity row. Empty values fall back to the brand layer.
ALTER TABLE email_settings ADD COLUMN logo_url TEXT NOT NULL DEFAULT '';
ALTER TABLE email_settings ADD COLUMN brand_color TEXT NOT NULL DEFAULT '';
ALTER TABLE email_settings ADD COLUMN footer_text TEXT NOT NULL DEFAULT '';
ALTER TABLE email_settings ADD COLUMN footer_address TEXT NOT NULL DEFAULT '';
ALTER TABLE email_settings ADD COLUMN footer_links TEXT NOT NULL DEFAULT '[]';
ALTER TABLE email_settings ADD COLUMN show_powered_by INTEGER NOT NULL DEFAULT 0;
