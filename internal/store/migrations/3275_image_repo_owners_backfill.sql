-- Claim image_repo_owners for existing apps from their current image repo.
-- Oldest app (lowest rowid) wins a contested repo, later ones are skipped.
WITH base AS (
    SELECT rowid AS ord, name,
           CASE
               WHEN instr(image, '@') = 0
                    AND length(image) > 41
                    AND substr(image, length(image) - 40, 1) = ':'
                    AND substr(image, length(image) - 39) NOT GLOB '*[^0-9a-f]*'
                   THEN lower(substr(image, 1, length(image) - 41))
               ELSE lower(name)
           END AS repo
    FROM desired_services
), stripped AS (
    SELECT ord, name,
           CASE
               WHEN repo LIKE 'index.docker.io/%' THEN substr(repo, 18)
               WHEN repo LIKE 'docker.io/%' THEN substr(repo, 11)
               ELSE repo
           END AS repo,
           (repo LIKE 'index.docker.io/%' OR repo LIKE 'docker.io/%') AS hub
    FROM base
), normalized AS (
    SELECT ord, name,
           CASE
               WHEN NOT hub AND instr(repo, '/') > 0
                    AND (substr(repo, 1, instr(repo, '/') - 1) LIKE '%.%'
                         OR substr(repo, 1, instr(repo, '/') - 1) LIKE '%:%'
                         OR substr(repo, 1, instr(repo, '/') - 1) = 'localhost')
                   THEN repo
               WHEN instr(repo, '/') = 0 THEN 'docker.io/library/' || repo
               ELSE 'docker.io/' || repo
           END AS repo
    FROM stripped
)
INSERT OR IGNORE INTO image_repo_owners (repo, app_name)
SELECT repo, name FROM normalized WHERE repo <> '' ORDER BY ord;
