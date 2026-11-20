-- Lets a deploy find the record of an image by digest or repo, wherever the image is about to run.
ALTER TABLE build_records ADD COLUMN digest TEXT NOT NULL DEFAULT '';
ALTER TABLE build_records ADD COLUMN repo TEXT NOT NULL DEFAULT '';
UPDATE build_records SET
    digest = COALESCE(json_extract(claims, '$.digest'), ''),
    repo = COALESCE(json_extract(claims, '$.repo'), '')
WHERE json_valid(claims);
CREATE INDEX build_records_digest ON build_records(digest);
CREATE INDEX build_records_repo ON build_records(repo, created_at);
