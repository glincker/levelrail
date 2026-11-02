# Migration notes for stack shop

Generated from config files (vercel). Review the documents in a pull request, then run `levelrail-cli stack plan shop -f .` before the first apply.

## What the files do not do

- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.
- Secret values, volumes and database contents are not carried over by the documents.
- [vercel.build.custom] vercel.json: Railpack runs the package.json build and start scripts, custom build, install and output settings are not applied Fix: Move them into package.json scripts, or use a Dockerfile and build.type dockerfile.
- [vercel.build.env] vercel.json build.env: vercel.json build.env is not read, only app env vars with a public prefix (NEXT_PUBLIC_, VITE_ and similar) reach a build Fix: Set the NEXT_PUBLIC_* values as plain app env before the first deploy; secrets are never passed to a build.
- [vercel.regions.multi] vercel.json iad1,fra1: no equivalent: multi-region deployment, a service runs on one node Fix: Pick the node closest to your users.
- [vercel.runtime.edge] vercel.json app/api/og/route.ts: no equivalent: Vercel Edge runtime Fix: Move this code to the Node runtime, then drop the runtime setting.
- [vercel.function.maxduration] vercel.json app/api/report/route.ts: no per-function time limit applies in a container Fix: Remove maxDuration, or enforce a timeout in the app.
- [vercel.function.memory] vercel.json: the largest function memory is applied to the whole service Fix: Adjust resources.memory to what the whole app needs.
- [vercel.routing.server] vercel.json rewrites/redirects/headers: routing and header rules only apply to static services, this one runs a server Fix: Move them into the app (for Next.js, next.config.js rewrites, redirects and headers move with the app).
- [vercel.cron.http] vercel.json /api/cron/daily: Vercel cron makes an HTTP call, a Levelrail task runs a command in the container, so a curl, wget or node call is proposed that sends Authorization: Bearer $CRON_SECRET when CRON_SECRET is set Fix: Set CRON_SECRET as an app secret if the route checks it, and confirm the port is right.
- [vercel.cron.http] vercel.json /api/cron/daily?full=1: Vercel cron makes an HTTP call, a Levelrail task runs a command in the container, so a curl, wget or node call is proposed that sends Authorization: Bearer $CRON_SECRET when CRON_SECRET is set Fix: Set CRON_SECRET as an app secret if the route checks it, and confirm the port is right.

## App web

- builds from source (type railpack): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo
