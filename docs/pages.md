---
title: "Static pages"
sidebarTitle: "Pages"
description: Deploy static HTML sites directly from git with no container, served by the embedded ingress with full TLS and domain routing.
"og:url": "https://docs.levelrail.com/pages"
---

## What this covers

A static page is a site deployed directly from a git repository with no running container. Levelrail Pages fetches your code, optionally builds it in a sandboxed build container, publishes the output to a local directory, and routes traffic to it through the embedded Caddy ingress. This page covers the build step, deployment, domains, rollback, and deletion.

## Deploy a static site from git

In `app.yaml`, set `build.type: static` and optionally `build.path` to point to the directory containing your finished HTML:

```yaml
version: 1
services:
  web:
    build:
      type: static
      path: ./dist           # optional; defaults to the root of the repository
    domains:
      - example.com
      - www.example.com
```

The path must be relative and point to a directory that exists in your repository. Levelrail Pages serves that directory directly through Caddy's file_server handler, publishing the built files to a directory named after the commit so an earlier deploy can be re-published without a rebuild.

From the CLI, create the app from the spec file and say which repository and branch to deploy:

```bash
levelrail-cli apps create --name my-site --file app.yaml --service web \
  --repo https://github.com/you/site.git --ref main
```

From the dashboard, use the **New** wizard and pick the **Git repository** card. If you push an `app.yaml` with a static service, the deploy trigger will pick it up automatically.

## Create a site from a repository in one call

`POST /api/v1/static-sites` accepts an optional `git` object and `deploy`:

```json
{"name": "my-site", "git": {"repo_url": "https://github.com/org/site.git", "branch": "main", "account_id": "", "build_command": "npm run build", "output_dir": "dist", "deploy_on_push": true}, "deploy": true}
```

It creates the app, connects the source and starts the first build. The response is the site detail plus `git_source` and `deploy_id` (empty if the attempt did not exist within `APP_PAGES_DEPLOY_ID_WAIT`, default 3s). `deploy_on_push: false` sets `trigger_mode` to `manual`. A body with only `name` still works. From the CLI: `levelrail-cli pages deploy --git <url> [--branch b] [--build-command c] [--output-dir d]`.

### Which build settings win

For the build command and output directory, a per-site setting (`PATCH /api/v1/static-sites/{name}/settings`) wins when set, then the git source build config (`PUT /apps/{name}/git-source/build`), then detection. The per-site settings can be saved before the first publish. `trigger_mode` (`push`, `release` or `manual`) is set with `PUT /apps/{name}/git-source/deploy-settings`; `manual` ignores pushes and polls.

## Build a static site

Set `build.command` and `build.publish` to build the site on the control plane instead of committing the output:

```yaml
version: 1
services:
  web:
    build:
      type: static
      baseDirectory: apps/web      # optional; the project root inside the repo
      command: npm run build
      publish: dist                # relative to baseDirectory
      args:                        # optional build-time variables, e.g. VITE_*
        VITE_API_URL: https://api.example.com
    domains:
      - example.com
```

- `command` and `publish` go together, and `publish` replaces `build.path` (use one or the other).
- The build runs in a throwaway container from the toolchain image (`node:22-bookworm-slim` by default, override with `APP_STATIC_BUILD_IMAGE`). Dependencies are installed first with the package manager that matches your lockfile (`npm ci`, `pnpm install --frozen-lockfile`, `yarn install --frozen-lockfile`, or `npm install`). Per-build CPU, memory and time limits apply, the same as for container builds.
- `args` become build-time environment variables. Do not put secrets there, the output is public.
- The output is capped at `APP_STATIC_BUILD_MAX_MB` (default 2048).

Framework presets prefill the pair: Vite and Astro use `npm run build` with `dist`, a Next.js static export uses `out`, and Docusaurus uses `build`. In the dashboard, open the app's **Source** tab, choose **Static site**, and use the detected preset or type your own. From the CLI:

```bash
levelrail-cli apps git-source set my-site --build-type static --infer-build
levelrail-cli apps git-source set my-site --build-type static --build-command "pnpm build" --publish-dir build
```

`--infer-build` detects the framework and fills whichever of the command and publish directory you leave out.

## How static pages are built and deployed

Without `build.command`, there is no build step and your committed files are served as they are. With it, the build runs first. The deploy process is:

1. **Fetch**: Clone or pull the repository at the commit you push.
2. **Build** (only with `build.command`): Run the command in a build container and collect `build.publish` into a staging directory. A failed build or an empty output stops the deploy before anything changes.
3. **Publish**: Copy the specified `build.path` directory (or move the built output) to a local directory on the control plane, in one step. The files are stored in a content-addressed path under the control plane's data directory (`<data dir>/static/SERVICE/COMMIT`, with the data directory defaulting to `/var/lib/levelrail-data`).
4. **Route**: The ingress controller points the configured domains at this directory. Traffic routes immediately once the files are published.

If the deploy fails (e.g., the path does not exist in the repository), the previous version stays live until you fix and push again.

## Call another app from your site

A rule such as `/api/* app://my-api/:splat 200` in `_redirects`, `app.yaml` or the dashboard forwards `/api/*` to the app `my-api` on this Levelrail, so the browser talks to one origin and needs no CORS. Only an app name is accepted, the site password gate stays in front, and a missing target answers 502. Proxied responses carry no cookies, the request body is capped, and a slow target answers 502. Details and limits are in [Static site routing rules](/static-routing#proxy-a-path-to-another-app).

## Domains and TLS

Static sites declare domains the same way container apps do, in the `domains` list or from the **Domains** tab:

```yaml
domains:
  - example.com
  - www.example.com
```

Every domain gets an automatic TLS certificate from Let's Encrypt. The ingress refreshes certificates automatically well before expiry.

See [Domains and ingress](/domains-and-ingress) for domain verification, wildcard support, and custom proxies.

## Split testing

A static site with an explicit domain can run a [split test](/split-testing) between production and retained deploys, and promote the winner. See [Split testing](/split-testing) for the 409 you get when the site only has the automatic host.

## Rollback a static site

Levelrail Pages keeps the built files from every deploy by storing them with the commit SHA as part of the path. To roll back:

```bash
levelrail-cli apps deploys rollback-to my-site DEPLOY_ID
```

Or in the dashboard, click **Rollback** on a previous deploy in the **Deployments** view. No rebuild is needed: Levelrail re-publishes the earlier tree and points the route at it.

Rollback works by content digest: each deploy records a hash of the published tree, and rolling back re-publishes the earlier tree if its files are still on disk and unchanged (it answers 410 when the tree is gone and 409 when its content changed).

## Delete a static site

`pages delete` removes the site together with its backing app, its files and its route right away:

```bash
levelrail-cli pages delete my-site
```

`apps delete my-site` does the same. It needs the ability to delete the app.

To remove only the published site and keep the app record, use `levelrail-cli static-sites delete my-site`. It returns 404 if the site does not exist and 409 if the app still needs it.

If a site is ever left behind without an app, a background sweep removes it after a grace period. The default is 10 minutes. Set `APP_STATIC_ORPHAN_GRACE` (for example `1h`) to change it. An empty or `0` value falls back to the default.

Every deploy writes a release directory. After a successful publish, Levelrail keeps the newest 10 releases of a site and deletes older ones. Set `APP_PAGES_KEEP_DEPLOYS` to change the count (minimum 3; the older name `APP_STATIC_KEEP_RELEASES` still works). The live release, the previous one (the rollback target), releases used by queued, running, held or awaiting approval deploys, and releases another site still serves are never deleted, and nothing is deleted while a publish lock is set.

## Deploy URLs and promote

Every retained deploy is served at its own URL, `<deployId8>--<site hostname>`: the first 8 letters or digits of the deploy id, two hyphens, then the site's automatic hostname (for example `ab12cd34--docs.203-0-113-9.sslip.io`; in local development `ab12cd34--docs.localhost` on the bound port). The files behind it never change, so you can review a deploy before it goes live. The site's own domains keep serving the promoted deploy.

Deploy URLs follow the retention count above and disappear when a deploy's files are pruned. They need automatic hostnames, so none are served when those are off. A password-protected site gates its deploy URLs with the same password.

```bash
levelrail pages deploys docs
levelrail pages promote docs DEPLOY_ID
```

`GET /api/v1/static-sites/{name}/deploys` lists the deploys with `url`, `live` and `promotable`. `POST /api/v1/static-sites/{name}/deploys/{deployId}/promote` makes one live: it needs the same permission as a rollback, checks that the files still match the recorded content digest, and answers 410 when they were pruned.

Rollback and promote follow the production history, not the order of the deploy list. A promote or rollback adds a new deploy marked live that serves the chosen deploy's files. `pages rollback <site>` with no id goes back to the production deploy that was live before the current one became live, so after promoting an older deploy it restores the version you had before the promote. Run it again and it returns to the deploy you rolled back from. With an explicit id it restores exactly that deploy.

## Resize images on request

When `pages.image_resize` is on, a request for a JPEG or PNG file with a `w` query value is answered at that width, keeping the aspect ratio, for example `/img/hero.jpg?w=640`. It is off by default, because decoding images uses memory. Other file types, a missing or non-numeric `w`, an image already smaller than the width, and files over the limits below are served unchanged. Resized copies are cached in memory, oldest dropped first. All keys are runtime settings and apply on the next request (see [runtime settings](/runtime-settings)).

| Setting | Default | Range | Meaning |
| --- | --- | --- | --- |
| `pages.image_resize` | off | on or off | Turn the `?w=` route on. |
| `pages.image_resize_max_width` | 2048 | 16 to 16384 | Widest size a request may ask for. A larger `w` is served at this width. |
| `pages.image_resize_max_source_bytes` | 10485760 | 1024 to 1073741824 | Largest source file, in bytes, that is decoded. |
| `pages.image_resize_max_pixels` | 40000000 | 10000 to 2147483647 | Largest source image in pixels that is decoded. |
| `pages.image_resize_cache_bytes` | 67108864 | 1048576 to 4294967296 | Memory held for resized copies. |

## Build logs and dependency cache

Each deploy keeps its build log, with secrets masked the same way as app deploy logs. Read it with `levelrail pages logs docs [DEPLOY_ID]` (latest deploy when omitted) and add `--follow` to stream a running build.

`GET /api/v1/static-sites/{name}/deploys/{deployId}/log` returns JSON lines with the phase each ran in (`?tail=N` keeps the last N). `GET .../log/stream` is the live SSE stream, the same one app deploys use. Both need read access to the site.

Static builds keep the npm, pnpm or yarn download cache per site in a BuildKit cache mount, so a rebuild skips re-downloading dependencies. It follows the existing build cache settings: the per-app cache switch (`build.cache_per_app`), the cache budget and the per-app cache clear all cover it.

## List your static sites

From the CLI:

```bash
levelrail-cli static-sites list
levelrail-cli static-sites list --json
```

From the API:

```bash
curl -H "Authorization: Bearer TOKEN" https://levelrail.example.com/api/v1/static-sites
```

This shows every site currently served through the embedded ingress, with no build or runtime information: just the site name and its domains. For the full view including recent deployments and settings, use the **Apps** list in the dashboard.

## Previews and branch deploys

A static site you deploy with `--ref` is one deploy of one ref. To get a stable URL per branch with automatic rebuilds on each push, connect the repository as a source-connected app and turn on [branch deploys](/preview-environments#branch-deploys). Pull request previews are separate and need a pull request.

### Pull request previews

A source-connected static site gets a preview for each pull request when previews are on. The pull request's head commit is built and published as its own site, `<site>-pr-<number>`, on the preview URL, and the pull request gets the usual comment and commit status. The live site, its domain and its release history are not touched. Closing or merging the pull request, or the stale preview sweep, removes the preview's files. The basic auth gate (`--gate`) is created and enforced on static previews, the same as on container previews. If the gate cannot be created, the preview is not published and is marked failed. A forward-auth gate on the preview domain is refused, because static routes cannot enforce it. See [static sites in previews](/preview-environments#static-site-previews).

## App env in static builds

A static build runs in a throwaway container, so app env does not reach it by default. Levelrail passes the literal env vars of the app whose names start with `VITE_`, `NEXT_PUBLIC_`, `PUBLIC_` or `GATSBY_`, because those frameworks copy them into the published files. Secrets, references to other resources and Vault values are never passed, and neither is anything outside the allowed prefixes. Values in `build.args` win over env of the same name. Set `APP_STATIC_BUILD_ENV_PREFIXES` (comma separated) to change the prefixes, or to an empty value to pass nothing.

Anything a build puts in the published directory is public. Do not give a `PUBLIC_` variable a value you would not print on the page.

## Front a static site with Cloudflare

With a Cloudflare DNS integration connected, Levelrail creates a domain's record as proxied when the server-wide proxy setting is on. The Cache tab of an app has a Cloudflare proxy switch that overrides it for that app alone (`GET` and `PUT /api/v1/apps/{name}/cdn`, body `{"proxied": true | false | null}`, where `null` goes back to the server setting). The change applies to records written afterwards; run the domain's DNS fix to update an existing record. Keep the Cloudflare SSL/TLS mode on Full (strict).

## Password protection

A site can be gated with one shared password. Visitors get the browser's sign-in prompt (user name `site`). Set it in the site's settings in the dashboard, with `levelrail pages settings set docs --password-stdin`, or by sending `password` to `PATCH /api/v1/static-sites/{name}/settings`; an empty string clears it. The password is stored only as a bcrypt hash and no API returns it, only `protected: true`. It covers the site's domains and its deploy URLs, and changing it needs the `write:sensitive` ability.

Length bounds come from `APP_PAGES_PASSWORD_MIN` (default 8) and `APP_PAGES_PASSWORD_MAX` (default 72, the bcrypt limit). A domain that has its own basic auth keeps that credential.

## Deploy from CI

A CI pipeline should not hold an admin token. Create a token that can only deploy one site:

```bash
levelrail pages token create docs --label github-actions
```

The token is shown once and stored hashed. It can upload an archive to that site, start the deploy and read the deploy status, and nothing else: other sites, settings, secrets and every other route answer 403. Expiry defaults to the token lifetime setting; pass `--expires-days N` to choose. `levelrail pages token list docs` shows tokens without secrets and `levelrail pages token revoke docs <id>` cuts one off. The dashboard has the same controls in the site's deploy area. The CLI reads the token from `APP_API_TOKEN` and the server from `APP_API_URL`, so the commands below need no extra flags.

GitHub Actions, using the action in this repository. Pin the action to a release tag or a full commit SHA, never a moving branch, and set `version` so the CLI matches. The action ships with v1.5.7, so it needs that release or a later one:

```yaml
- uses: actions/checkout@v4
- uses: GLINCKER/levelrail/actions/pages-deploy@v1.5.7
  with:
    version: v1.5.7
    site: docs
    dir: ./dist
    server: https://levelrail.example.com
    token: ${{ secrets.LEVELRAIL_PAGES_TOKEN }}
```

GitLab CI:

```yaml
deploy:
  image: alpine:3
  script:
    - apk add --no-cache curl
    - curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/install-cli.sh
    - curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/checksums.txt
    - grep ' install-cli.sh$' checksums.txt | sha256sum -c -  # macOS: shasum -a 256 -c -
    - sh install-cli.sh
    - levelrail-cli pages deploy ./dist --name docs
  variables:
    APP_API_URL: https://levelrail.example.com
    APP_API_TOKEN: $LEVELRAIL_PAGES_TOKEN
```

Plain curl, with a zip of the site:

```bash
curl -fsS -X POST "$APP_API_URL/api/v1/apps/docs/uploads" \
  -H "Authorization: Bearer $APP_API_TOKEN" \
  -H "Content-Type: application/zip" --data-binary @site.zip
```

Then start the deploy with `POST /api/v1/apps/docs/builds` using `upload_hash` from the response and `build.type: static`, and poll `GET /api/v1/apps/docs/deploys/{id}` until it is `succeeded`.

## From the CLI

```bash
levelrail pages deploy ./dist --name docs --domain docs.example.com
levelrail pages list
levelrail pages show docs
levelrail pages rollback docs
levelrail pages domains status docs docs.example.com
```

`pages deploy` zips the folder (skipping `.git`, `node_modules` and dotfiles other than `_redirects`, `_headers` and `.well-known`), creates the site if it is missing, uploads, publishes and prints the live URL. A `.zip` works too. `levelrail deploy ./dist` prints a hint toward `pages deploy` when the folder has an `index.html` and no Dockerfile or compose file.

### Start from a template

`levelrail pages init [dir] [--starter blank|landing|docs|portfolio]` writes `index.html`, `404.html`, `_redirects`, `_headers` and an `app.yaml` with `build.type: static` into the folder (default: the current one). Existing files are never overwritten. The starters match the dashboard gallery. Then run `levelrail pages deploy <dir>`.

### Deploy from a URL

`levelrail pages deploy --url https://example.com/site.zip --name docs` makes the control plane download a `.zip` or `.tar.gz`, check it and publish it, creating the site on the first run. The same works over the API: `POST /api/v1/static-sites` accepts `source_url`, and `POST /api/v1/apps/{name}/builds` accepts `source_url` with `build.type: static` to redeploy an existing site. It needs the same permission as a normal static deploy.

- Only `https` URLs without credentials are accepted. Private, loopback and link-local addresses are refused, including after redirects (at most 3, https only).
- The download is capped by `APP_PAGES_URL_MAX_MB` (default 100) and `APP_PAGES_URL_TIMEOUT` (default 60s).
- `APP_PAGES_URL_ALLOW_PRIVATE` (default off) lets the fetch reach private addresses; it is independent of other outbound settings. `APP_PAGES_URL_CONCURRENCY` (default 4) bounds parallel downloads and `APP_PAGES_URL_STORE_MAX_MB` (default 5120) refuses new downloads once stored archives reach that size.
- A failed fetch returns one generic message; details stay in the server log, without query strings. A URL deploy is audited by host and path only, and only a static site can be deployed this way.
- Archives with absolute paths, `..` segments, symlinks or special files are rejected. The file count and unpacked size limits are the upload limits (`APP_UPLOAD_MAX_FILES`, `APP_UPLOAD_MAX_EXTRACTED_BYTES`).
- A zip with a single wrapper folder is served from inside that folder. A `.tar.gz` is served as packed.

## Domain checklist

`GET /api/v1/static-sites/{name}/domains/{domain}/status` also returns a `diagnosis` list of `{code, ok, hint}` rows: DNS pointing at this host (A, AAAA, CNAME, CDN proxy), propagation, ports 80 and 443, certificate and CA failure reason, a private or CGNAT host address, and whether `www` and the apex agree. It reuses the [domain doctor](/domain-diagnosis), so the same time limits apply (`APP_DOMAIN_DOCTOR_TIMEOUT`, `APP_DOMAIN_DOCTOR_PROBE_TIMEOUT`). The diagnosis is cached per site and domain for `APP_PAGES_DOCTOR_CACHE_TTL` (default 30s) and shared by concurrent callers; a stale one is returned at once and refreshed in the background. Add `?diagnose=1` to force a fresh run (`pages domains status` does). `verified` and `cert` are always live. A missing `www` or apex record is shown as an optional hint, not a failure. Port checks only run when the domain resolves to this server. The dashboard shows the rows as a compact checklist on the domain card and `pages domains status` prints them.

## Contact forms

A plain HTML form on a site can post to `/_forms/<form-name>` on the site's own host, so no CORS setup is needed. Create the form first with `levelrail-cli forms create <site> <form-name>` (redirect, honeypot field and notification channels are set there), then point the form at it:

```html
<form method="post" action="/_forms/contact">
  <input name="email" type="email" required>
  <input name="bot-field" hidden tabindex="-1" autocomplete="off">
  <button>Send</button>
</form>
```

Browser posts redirect to the form's redirect URL or show a built-in thank-you page; `fetch()` with `Accept: application/json` gets `{"ok":true}`. Spam is dropped quietly (honeypot, link count), and posts are limited per IP and by body size. The limits and retention come from `FORMS_RATE_PER_IP_PER_MINUTE`, `FORMS_MAX_BODY_BYTES` and `FORMS_RETENTION_DAYS`. Submissions keep a salted hash of the sender address, never the address. Read them in the site's Forms tab or with `levelrail-cli pages submissions <site>`; `GET /api/v1/static-sites/{name}/submissions` returns them across forms. A password-protected site keeps the gate in front of the form route.

### Export submissions as CSV

In the site's Forms tab, each form has an **Export <form> as CSV** button. It downloads `GET /api/v1/apps/{name}/forms/{form}/export` (ability `read:sensitive`), a `text/csv` attachment named `<app>-<form>-submissions.csv` with up to 50,000 submissions. The columns are `id`, `created_at` (UTC, RFC3339) and `spam`, then one column per field name seen across the submissions, sorted by name. Cells that a spreadsheet would run as a formula are prefixed with a quote. Submissions marked as spam show a **Spam** badge in the tab, with the reason when one was recorded.

### Preview the 404 page

The site's Preview has a **Preview 404 page** toggle. It loads a path that no site serves (`/__missing-page-preview__`) on the site's domain, so you see your own `404.html` as visitors would. The toggle is disabled until the site has a domain.

## Deploy hook

A git-connected site can have a secret URL that redeploys its branch when anything POSTs to it, such as a CMS publish, a CI job or a cron. Create it in the site's settings or with `levelrail-cli pages hook create <site>`; the URL is shown once and only its hash is stored. `pages hook rotate` replaces it and the old URL stops at once, `pages hook rm` revokes it. The API takes `POST /api/v1/static-sites/{name}/hook` to create (`409` when one exists) and the same call with `{"rotate": true}` to rotate (`409` when none exists), so a stale view cannot replace a hook by accident. Disconnecting the git source or deleting the app removes the hook, and a hook made before the current git source was connected stops working.

```bash
curl -X POST https://levelrail.example.com/api/v1/hooks/pages/docs/<secret>
```

The call answers `202` with the deploy id when the deploy started, `409` while a deploy is already running, a deploy of a protected environment awaits approval, or deploys are frozen, `429` when called too often (limits shared with build hooks) and `404` for an unknown site or a wrong secret. Calls are audited without the secret and with the hook's creation time, so rotations are told apart; the first call refused for rate in a window is audited, wrong secrets are not. Sites without a git source cannot have a hook. `GET /api/v1/static-sites/{name}/hook` returns only `{enabled, created_at}`.

## Deploy notifications

A Pages site sends a message when a deploy succeeds or fails, through the notification channels you already connected (email, chat, webhook, push). Channel secrets and the destination checks stay in the channel service, so a site never holds a webhook URL itself.

```bash
levelrail-cli pages notify set my-site --channel chn_abc123 --on failure
levelrail-cli pages notify show my-site
levelrail-cli pages notify off my-site     # mute without detaching channels
levelrail-cli pages notify reset my-site   # back to every outcome
```

`--on` takes `success`, `failure` or both, comma separated. Without a stored filter a site notifies on every outcome of its attached channels. The API is `GET`, `PUT` and `DELETE /api/v1/static-sites/{name}/notify`; the response lists attached channel IDs and never their destinations.

## Traffic over time

The site page shows a small requests chart with a 24 hour, 7 day and 30 day toggle. `GET /api/v1/static-sites/{name}/traffic?range=24h|7d|30d` returns a `series` array of buckets, each with `start`, `requests`, `pages`, `assets` and `errors` (4xx and 5xx). Recent buckets come from the edge access log and older hours from the stored hourly rollups, so history survives a restart. A missing path that a broken link points to also shows "Linked from" with the pages that link to it. Each `broken_link` finding from `GET /api/v1/static-sites/{name}/checks` carries `path` (the page with the link) and `target` (the link that does not resolve).

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_PAGES_TRAFFIC_BUCKETS` | 24 | Buckets per series, clamped to 2 to 120 |
| `APP_PAGES_TRAFFIC_RETENTION_DAYS` | 30 | Longest range the endpoint serves |

## Smoke test

`scripts/pages-smoke.sh` exercises a running Levelrail end to end: deploy, clean URLs, redirects, headers, custom 404, per-deploy URLs, promote and rollback, the password gate, checks, traffic and an optional git deploy. It prints PASS, FAIL or SKIP per step and exits nonzero on any failure. Features the server does not have yet are skipped, not failed. It is not part of CI because it needs a live instance.

```bash
LEVELRAIL_URL=http://127.0.0.1:8190 LEVELRAIL_TOKEN=... scripts/pages-smoke.sh
SMOKE_DRY_RUN=1 scripts/pages-smoke.sh   # print the plan only
```

Optional: `SMOKE_HOST` (address to connect to for a real host), `SMOKE_GIT_URL` (public repo for the git step), `SMOKE_DOMAIN` (domain status step), `LEVELRAIL_CLI` (use a built binary instead of building one). It needs `curl` and `jq`, and always deletes its temp site on exit.

## Limitations

- **Build toolchain**: the build container is Node based. For other generators (Hugo, Jekyll), set `APP_STATIC_BUILD_IMAGE` to an image that has the tool and a shell, or build in CI and serve the committed output with `build.path`.
- **Routing**: redirects, rewrites, headers, single page app fallback, a custom 404 page and clean URLs (on by default, so `about.html` serves at `/about`) come from `_redirects`, `_headers` and `app.yaml` and are published with each deploy. See [Static site routing rules](/static-routing). Proxying a path to another origin and edge conditions (country, language) are not supported; use a path forwarder on the domain for proxying.
- **Large file limits**: There are no hard limits, but very large directories (> 1GB) may see slower deploy and rollback times. Monitor the data directory disk usage with `df -h /var/lib/levelrail-data`.

## For contributors

<Accordion title="Where static pages live in the source">
- Deploy logic: `internal/deploy/static.go`
- Storage: `internal/store/static_site.go`, migrations `0015_static_sites.sql`
- Ingress routing: `internal/reconcile/ingress/controller_static_test.go` (controller tests; no separate impl file)
- Routing rules: `internal/staticroutes` (parsers and resolver), `internal/deploy/static_routes.go` (compile at deploy time), `internal/ingress/static_site.go` (serving)
- API: `internal/api/static_sites.go`, `internal/api/static_routes.go`
- CLI: `cmd/levelrail-cli/static_sites.go`
</Accordion>
