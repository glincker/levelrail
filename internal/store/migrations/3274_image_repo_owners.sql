-- One owner app per normalized image repository, claimed at build time.
CREATE TABLE image_repo_owners (
    repo     TEXT PRIMARY KEY,
    app_name TEXT NOT NULL
);
CREATE INDEX image_repo_owners_app ON image_repo_owners(app_name);
