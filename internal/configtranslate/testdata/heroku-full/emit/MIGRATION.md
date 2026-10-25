# Migration notes for stack shop

Generated from config files (heroku). Review the documents in a pull request, then run `levelrail-cli stack plan shop -f .` before the first apply.

## What the files do not do

- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.
- Secret values, volumes, scheduled jobs and database contents are not carried over by the documents.
- [heroku.buildpack.unsupported] app.json https://github.com/example/custom-buildpack: no equivalent: buildpack https://github.com/example/custom-buildpack is not a Railpack provider Fix: Write a Dockerfile that does what the buildpack did and use build.type dockerfile.
- [heroku.process.port] Procfile clock: the spec requires a port on every non-static service, nothing needs to listen on it Fix: Leave it as is, or add an exec health probe.
- [heroku.process.custom] Procfile clock: process type clock becomes its own always-on service Fix: If it is a scheduler loop, move the work to jobs with a cron schedule.
- [heroku.web.port] Procfile web: Heroku injects $PORT, here the service listens on a fixed port and PORT is set to match Fix: Change port and the PORT env value together if your app expects another port.
- [heroku.formation.size] app.json web: dyno size standard-1x has no direct mapping Fix: Set resources.memory from the dyno's RAM, or leave it unlimited.
- [heroku.process.port] Procfile worker: the spec requires a port on every non-static service, nothing needs to listen on it Fix: Leave it as is, or add an exec health probe.
- [heroku.release.predeploy] Procfile release: release phase becomes hooks.preDeploy, run in the new web container before readiness, not in a one-off dyno Fix: Make sure the command is idempotent and needs nothing but the built image.
- [heroku.script.postdeploy] app.json postdeploy: Heroku runs postdeploy only when a review app is created, hooks.postDeploy runs on every deploy Fix: Drop it, or make the command safe to repeat.
- [heroku.script.unsupported] app.json test: no equivalent: app.json script test is not translated Fix: Move test scripts to CI and teardown work to a manual step.
- [heroku.addon.database] app.json heroku-postgresql:essential-0: add-on maps to a managed postgres database named main, exposed as DATABASE_URL Fix: Dump the data from Heroku and restore it into the new database before cutover.
- [heroku.addon.database] app.json heroku-redis:mini: add-on maps to a managed redis database named cache, exposed as CACHE_URL Fix: Dump the data from Heroku and restore it into the new database before cutover.
- [heroku.addon.logging] app.json papertrail:choklad: logs are stored and searchable on the node, no add-on needed Fix: Drop this add-on, or ship logs elsewhere with a drain if you must.
- [heroku.addon.scheduler] app.json scheduler:standard: Heroku Scheduler jobs live in the add-on, not in app.json Fix: Recreate each job under the service's jobs: with a cron schedule.
- [heroku.addon.manual] app.json sendgrid:starter: no equivalent: add-on sendgrid is not in the Levelrail catalog Fix: Sign up with the vendor directly or self-host it, then set its env vars by hand.
- [heroku.env.unset] app.json OPTIONAL_FLAG: optional with no default, not proposed Fix: Add it to the service env if the app reads it.
- [heroku.env.generator] app.json SECRET_KEY_BASE: Heroku generated this value, Levelrail does not Fix: Generate one (for example openssl rand -hex 32) and set it with apps secrets set.
- [heroku.env.required] app.json STRIPE_KEY: required with no default, proposed as a required secret Fix: Set it with apps secrets set before the first deploy.
- Databases are created empty. Copy the data in with `levelrail-cli migrate db-copy`.

## App clock

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo

## App web

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo

## App worker

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo
