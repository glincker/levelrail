-- Teams reuse the organizations table. Existing installs get one default team
-- owning every project, and every existing user joins every existing team.
INSERT OR IGNORE INTO organizations (id, name) VALUES ('org_default', 'Default');

UPDATE projects SET org_id = 'org_default' WHERE org_id IS NULL;

CREATE TABLE team_members (
    team_id    TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'developer', 'viewer')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX idx_team_members_user_id ON team_members (user_id);

-- Owner only for instance admins: a read-only user made owner could invite
-- others with more than they hold. Deployers become admin so protected
-- environments keep accepting their deploys.
INSERT INTO team_members (team_id, user_id, role)
SELECT o.id, u.id,
    CASE
        WHEN u.abilities LIKE '%"root"%' THEN 'owner'
        WHEN u.abilities LIKE '%"deploy"%' THEN 'admin'
        WHEN u.abilities LIKE '%"write"%' THEN 'developer'
        ELSE 'viewer'
    END
FROM users u CROSS JOIN organizations o;
