-- A parked domain stays registered to the instance (its DNS records and
-- certificate are untouched) but is routed by no app. domainOwner treats a
-- row here as an owner so nothing else can claim the hostname until it is
-- attached to an app again or deleted.
CREATE TABLE parked_domains (
    domain      TEXT PRIMARY KEY,
    parked_from TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    parked_by   TEXT NOT NULL DEFAULT '',
    parked_at   TEXT NOT NULL
);
