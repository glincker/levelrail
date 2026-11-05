-- Stack scope: a project stack belongs to one project, an install stack owns
-- install-wide documents and has no project (ADR 077).
ALTER TABLE stacks ADD COLUMN scope TEXT NOT NULL DEFAULT 'project'
    CHECK (scope IN ('project', 'install') AND (scope = 'project' OR project_id = ''));
