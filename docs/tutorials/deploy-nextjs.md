---
title: "Deploy a Next.js app to your own server"
icon: "/assets/icons/deploy.svg"
sidebarTitle: "Deploy Next.js"
description: Deploy a Next.js app from a git repository to a server you run, with a domain, HTTPS, a health check and a rollback.
"og:url": "https://docs.levelrail.com/tutorials/deploy-nextjs"
---

This tutorial takes a Next.js app from a git repository to a domain on your own server. It builds with Railpack, which is the default for a Next.js repository, and it ends with a rollback you can run in one command.

For the full reference on Next.js, including the Dockerfile route and the static export, see [Self-host Next.js](/self-host-nextjs) and [Frameworks](/frameworks).

Performance numbers are pending a host run. The sizing guidance in [Build performance](/build-performance#sizing-a-vps) is arithmetic from assumed memory per site, not a measurement.

## Before you start

- A running Levelrail instance and the CLI logged in to it ([installing](/installing)).
- A Next.js app in a git repository that your server can clone. The app must start with `npm run start` after `npm run build`, which is the Railpack default.
- A domain name you control, pointed at the server. See [Domains and ingress](/domains-and-ingress).
- A server with at least 2 GB of RAM for a build. Building on a 1 GB host is not recommended. See [Build performance](/build-performance).

## Step 1: Create the app from the repository

Pick the port Next.js listens on. The default is 3000, and `next start` uses it unless `PORT` is set.

```bash
levelrail-cli up --repo https://github.com/you/your-next-app --ref main \
  --name web --port 3000 --domain web.example.com --dry-run
```

The dry run prints the plan without creating anything. Read it, then run the same command without `--dry-run`:

```bash
levelrail-cli up --repo https://github.com/you/your-next-app --ref main \
  --name web --port 3000 --domain web.example.com
```

Levelrail detects the Next.js framework, builds it with Railpack and starts it with `npm run start`. The build log says which framework it chose.

## Step 2: Set the public environment before the first build

Next.js inlines `NEXT_PUBLIC_*` values at `next build`, so a value set after the first build does not reach the browser until the next build. Add those values as plain app environment variables in the dashboard, on the app's **Environment** card, before you deploy. Do not put them in the secrets store, because secrets are never passed to a build.

Server-only values, such as a database URL, go in the same card as secrets. A database connection is covered in [Connect an app to Postgres](/tutorials/connect-an-app-to-postgres).

## Step 3: Set a health check

The health check decides whether a new release replaces the old one. For a Next.js app, `/` is a reasonable first check, but a route that does not touch the database is better, so a slow database does not block every deploy.

```bash
levelrail-cli apps health set web --probe readiness --path / --expected-status 200-399
```

Readiness gates the switch to a new release. Read [Zero-downtime deploys with health checks](/tutorials/zero-downtime-deploys-with-health-checks) for what a failed check looks like.

## Step 4: Check the deploy

```bash
levelrail-cli apps status web
levelrail-cli logs web --tail 50
```

Open `https://web.example.com` in a browser. The page should load with a valid certificate. If the certificate is not issued yet, the status shows the reason, and [Domains and ingress](/domains-and-ingress) explains the HTTP-01 and DNS-01 options.

## Step 5: Ship a second version and roll back

Push a change to the branch, or redeploy the same branch:

```bash
levelrail-cli apps deploys list web
levelrail-cli rollback web
```

`rollback` puts back the previous release. Check the page again. Whether a rollback rebuilds the image is pending run.

## Troubleshooting

- **The build runs out of memory (exit 137).** The build hit its memory cap. Raise the build limit for the app, or add swap on the server. The build log names the setting. See [Build performance](/build-performance#build-memory-guard).
- **The page is blank or shows old `NEXT_PUBLIC_` values.** The variable was added after the build. Redeploy so `next build` runs again.
- **The health check fails right after deploy.** Raise the readiness timeout, or check that the app listens on the port you set in Step 1.

## Where to go next

- [Self-host Next.js](/self-host-nextjs): the Dockerfile route, preview environments and the static export.
- [Deploy from GitHub Actions](/tutorials/deploy-from-github-actions): build in CI and deploy on every push.
