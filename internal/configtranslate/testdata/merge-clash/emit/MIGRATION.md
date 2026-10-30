# Migration notes for stack shop

Generated from config files (heroku, vercel). Review the documents in a pull request, then run `levelrail-cli stack plan shop -f .` before the first apply.

## What the files do not do

- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.
- Secret values, volumes and database contents are not carried over by the documents.
- [heroku.web.port] Procfile web: Heroku injects $PORT, here the service listens on a fixed port and PORT is set to match Fix: Change port and the PORT env value together if your app expects another port.
- [heroku.health.none] app.json web: no healthcheck is defined, so deploys are not readiness gated Fix: Add health.readiness with a cheap path such as /health.
- [heroku.process.port] Procfile worker: the spec requires a port on every non-static service, nothing needs to listen on it Fix: Leave it as is, or add an exec health probe.
- [vercel.build.custom] vercel.json: Railpack runs the package.json build and start scripts, custom build, install and output settings are not applied Fix: Move them into package.json scripts, or use a Dockerfile and build.type dockerfile.
- [vercel.cron.http] vercel.json /api/tick: Vercel cron makes an HTTP call, a Levelrail task runs a command in the container, so curl is proposed Fix: Confirm curl exists in the image and the port is right, or call the code directly with a script.
- [merge.service-clash] vercel.json web: both Heroku and Vercel config define this service, both are kept: the Heroku one as "web" and the Vercel one as "web-vercel"; its 1 cron job(s) stay on "web-vercel" Fix: Decide which service you still need. Both are emitted so nothing is silently dropped.

## App web

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo

## App web-vercel

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo

## App worker

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo
