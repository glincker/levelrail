-- Public URL operators use to reach the dashboard. When it is https,
-- plain-HTTP logins are refused (internal/api/dashboard_url.go).
ALTER TABLE ingress_settings ADD COLUMN dashboard_url TEXT;
