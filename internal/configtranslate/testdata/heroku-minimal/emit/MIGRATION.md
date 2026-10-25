# Migration notes for stack shop

Generated from config files (heroku). Review the documents in a pull request, then run `levelrail-cli stack plan shop -f .` before the first apply.

## What the files do not do

- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.
- Secret values, volumes, scheduled jobs and database contents are not carried over by the documents.
- [heroku.web.port] Procfile web: Heroku injects $PORT, here the service listens on a fixed port and PORT is set to match Fix: Change port and the PORT env value together if your app expects another port.
- [heroku.health.none] app.json web: no healthcheck is defined, so deploys are not readiness gated Fix: Add health.readiness with a cheap path such as /health.

## App web

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo
