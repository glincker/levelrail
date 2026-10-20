-- Lets an operator turn off the automatic <app>.<dashed-ip>.sslip.io
-- hostname apps without a domain get. Stored inverted so the default is on.
ALTER TABLE ingress_settings ADD COLUMN fallback_domains_disabled INTEGER NOT NULL DEFAULT 0;
