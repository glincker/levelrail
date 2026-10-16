---
title: "Static pages"
sidebarTitle: "Pages"
description: Deploy static HTML sites directly from git with no container, served by the embedded ingress with full TLS and domain routing.
---

## What this covers

A static page is an HTML site deployed directly from a git repository with no container involved. Levelrail Pages fetches your code, publishes the built files to a local directory, and routes traffic to them through the embedded Caddy ingress. No build packs or Dockerfiles needed. This page covers deployment, domains, rollback, and deletion.

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

## How static pages are built and deployed

Unlike container-based apps, there is no container build step. The deploy process is:

1. **Fetch**: Clone or pull the repository at the commit you push.
2. **Publish**: Copy the specified `build.path` directory to a local directory on the control plane. The files are stored in a content-addressed path under the control plane's data directory (`<data dir>/static/SERVICE/COMMIT`, with the data directory defaulting to `/var/lib/levelrail-data`).
3. **Route**: The ingress controller points the configured domains at this directory. Traffic routes immediately once the files are published.

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

## Limitations

- **Build languages**: Levelrail Pages does not run a build step. Your repository must contain the finished HTML in the path you specify. If you use a static site generator (Hugo, Jekyll, Next.js static export and so on), build in CI and push the output to a branch, then deploy that branch with `--ref`.
- **Redirects and rewrites**: The embedded Caddy serves files as-is. URL rewrites and sophisticated routing are not configured. If you need them, run a small container service for the site instead.
- **Large file limits**: There are no hard limits, but very large directories (> 1GB) may see slower deploy and rollback times. Monitor the data directory disk usage with `df -h /var/lib/levelrail-data`.

## For contributors

<Accordion title="Where static pages live in the source">
- Deploy logic: `internal/deploy/static.go`
- Storage: `internal/store/static_site.go`, migrations `0015_static_sites.sql`
- Ingress routing: `internal/reconcile/ingress/controller_static_test.go` (controller tests; no separate impl file)
- API: `internal/api/static_sites.go`
- CLI: `cmd/levelrail-cli/static_sites.go`
</Accordion>
