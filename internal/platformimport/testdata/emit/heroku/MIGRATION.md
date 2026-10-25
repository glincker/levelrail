# Migration notes for stack heroku-import

Generated from heroku. Review the documents in a pull request, then run `levelrail-cli stack plan heroku-import -f .` before the first apply.

## What the files do not do

- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.
- Secret values, volumes and database contents are not carried over by the documents.
- Databases are created empty. Copy the data in with the data copy step (dashboard: Settings, Import from another platform; CLI: migrate db-copy), which verifies row counts.
- heroku-postgresql add-on postgresql-round-1 on app shop uses plan heroku-postgresql:standard-0: pick a matching size here, data is not migrated
- heroku-redis add-on redis-deep-2 on app shop uses plan heroku-redis:premium-0: pick a matching size here, data is not migrated
- no managed equivalent, set up manually: sendgrid (sendgrid:starter on app shop)

## App bare

- no port found in the source, defaulted to 80
- Heroku keeps no repository for the app, its code only exists as slugs there
- Heroku injects PORT at runtime
- next: set the correct port in the app settings
- next: import the repository with `import <repo>` and attach it to this app
- next: make sure the app listens on the port set after import

## App shop

- no port found in the source, defaulted to 80
- Heroku keeps no repository for the app, its code only exists as slugs there
- Heroku injects PORT at runtime
- next: set the correct port in the app settings
- next: import the repository with `import <repo>` and attach it to this app
- next: make sure the app listens on the port set after import

## App shop-worker

- no port found in the source, defaulted to 80
- Heroku worker process of shop, dyno size Standard-2X, not exposed over HTTP
- Heroku keeps no repository for the app, its code only exists as slugs there
- next: set the correct port in the app settings
- next: create it as a worker from the same repository with the command from the Procfile
- next: import the repository with `import <repo>` and attach it to this app
