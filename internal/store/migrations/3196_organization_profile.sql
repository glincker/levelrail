-- Organization profile: an optional URL slug, a description and an avatar. The default
-- organization keeps a NULL slug. The avatar bytes live in their own table so listing
-- organizations never reads image data.
ALTER TABLE organizations ADD COLUMN slug TEXT;
ALTER TABLE organizations ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN avatar_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX idx_organizations_slug ON organizations(lower(slug)) WHERE slug IS NOT NULL;

CREATE TABLE organization_avatars (
    org_id     TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    mime       TEXT NOT NULL,
    bytes      BLOB NOT NULL,
    sha        TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
