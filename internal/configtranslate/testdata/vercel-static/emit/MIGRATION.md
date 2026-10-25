# Migration notes for stack shop

Generated from config files (vercel). Review the documents in a pull request, then run `levelrail-cli stack plan shop -f .` before the first apply.

## What the files do not do

- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.
- Secret values, volumes, scheduled jobs and database contents are not carried over by the documents.
- [vercel.route.pattern] vercel.json /gone/:id(\d+): pattern uses regex features the static router lacks, redirect skipped Fix: Rewrite it as literal paths, :name segments or a trailing /*.
- [vercel.route.condition] vercel.json /ab: has and missing conditions have no equivalent, redirect skipped Fix: Handle the condition in the app or drop the rule.
- [vercel.rewrite.proxy] vercel.json /api/(.*): no equivalent: rewrite to an external URL (proxy), rule skipped Fix: Proxy in the app, or point clients at the target directly.
- [vercel.cron.static] vercel.json crons: no equivalent: a static site has no container to run a scheduled task in Fix: Run the endpoints from a server service, or schedule them outside the site.

## App web

- 2 header rule(s) are not in the App document
- 4 route rule(s) (redirects and rewrites) are not in the App document
- static site (build "npm ci && npm run build", publish "dist"): serve the built files from an image, apply cannot build it
- the source names no port, 80 is assumed: set the port the image listens on
