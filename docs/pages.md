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

## Domains and TLS

Static sites declare domains the same way container apps do, in the `domains` list or from the **Domains** tab:

```yaml
domains:
  - example.com
  - www.example.com
```

Every domain gets an automatic TLS certificate from Let's Encrypt. The ingress refreshes certificates automatically well before expiry.

See [Domains and ingress](/domains-and-ingress) for domain verification, wildcard support, and custom proxies.

## Rollback a static site

Levelrail Pages keeps the built files from every deploy by storing them with the commit SHA as part of the path. To roll back:

```bash
levelrail-cli apps deploys rollback-to my-site DEPLOY_ID
```

Or in the dashboard, click **Rollback** on a previous deploy in the **Deployments** view. No rebuild is needed: Levelrail re-publishes the earlier tree and points the route at it.

Rollback works by content digest: each deploy records a hash of the published tree, and rolling back re-publishes the earlier tree if its files are still on disk and unchanged (it answers 410 when the tree is gone and 409 when its content changed).

## Delete a static site

Deleting the app removes the site, its files and its route right away:

```bash
levelrail-cli apps delete my-site
```

To remove only the published site and keep the app record, use `levelrail-cli static-sites delete my-site`. It returns 404 if the site does not exist and 409 if the app still needs it.

If a site is ever left behind without an app, a background sweep removes it after a grace period. The default is 10 minutes. Set `APP_STATIC_ORPHAN_GRACE` (for example `1h`) to change it. An empty or `0` value falls back to the default.

Every deploy writes a release directory. After a successful publish, Levelrail keeps the newest 10 releases of a site and deletes older ones. Set `APP_STATIC_KEEP_RELEASES` to change the count (minimum 3). The live release, the previous one (the rollback target), releases used by queued, running, held or awaiting approval deploys, and releases another site still serves are never deleted, and nothing is deleted while a publish lock is set.

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

A source-connected static site gets a preview for each pull request when previews are on. The pull request's head commit is built and published as its own site, `<site>-pr-<number>`, on the preview URL, and the pull request gets the usual comment and commit status. The live site, its domain and its release history are not touched. Closing or merging the pull request, or the stale preview sweep, removes the preview's files. Static previews do not enforce the preview basic auth gate. See [static sites in previews](/preview-environments#static-sites).

## Limitations

- **Build toolchain**: the build container is Node based. For other generators (Hugo, Jekyll), set `APP_STATIC_BUILD_IMAGE` to an image that has the tool and a shell, or build in CI and serve the committed output with `build.path`.
- **Routing**: redirects, rewrites, headers, single page app fallback, a custom 404 page and clean URLs come from `_redirects`, `_headers` and `app.yaml` and are published with each deploy. See [Static site routing rules](/static-routing). Proxying a path to another origin and edge conditions (country, language) are not supported; use a path forwarder on the domain for proxying.
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
