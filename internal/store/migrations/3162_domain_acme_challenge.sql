-- Per-host ACME challenge override: a host listed here gets its certificate
-- through DNS-01 while every other host keeps its own challenge.
CREATE TABLE domain_acme_challenge (
	domain       TEXT PRIMARY KEY,
	challenge    TEXT NOT NULL CHECK (challenge IN ('dns01')),
	requested_at TEXT NOT NULL
);
