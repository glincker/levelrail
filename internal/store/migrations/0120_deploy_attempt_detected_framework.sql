-- The framework the create-app-from-git wizard's pre-flight detection
-- (POST /api/v1/build/detect) reported before this attempt was
-- triggered, e.g. "Node.js" or "Go". Empty for every attempt triggered
-- without that pre-flight step (the CLI, a git-push webhook, any build
-- from before this column existed).
ALTER TABLE deploy_attempts ADD COLUMN detected_framework TEXT NOT NULL DEFAULT '';
