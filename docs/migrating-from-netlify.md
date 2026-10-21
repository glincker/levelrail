---
title: "Migrating a static site from Netlify"
description: "A runbook for moving a Netlify site to Levelrail: build settings, netlify.toml mapping, redirects and headers, deploy contexts, forms, build hooks, a DNS cutover and the features with no equivalent."
"og:url": "https://docs.levelrail.com/migrating-from-netlify"
---

This is a runbook for moving one Netlify site onto a Levelrail instance you run yourself. It covers only features that exist in this repository today. Where something has no equivalent, it says so. There is no Netlify importer, so every step is manual.

<Warning>
Some areas are still beta (see [feature status](/feature-status)), including [Forms](/forms). Keep the Netlify site live as your rollback path until the new one has run clean for a while.
</Warning>

## 1. Inventory what Netlify holds

Collect this before you touch Levelrail. Much of it lives in the Netlify dashboard, not in your repo.

- Build command, publish directory and base directory (Site configuration, Build and deploy).
- Environment variables, and which deploy contexts they apply to.
- `netlify.toml`, `_redirects` and `_headers`.
- Forms, with their notification settings.
- Build hooks and scheduled functions.
- Domains, DNS provider and current TTLs. Note whether the domain uses Netlify DNS.
- Use of Functions, Edge Functions, Identity, Blobs, Image CDN and Analytics. Section 5 covers each.

## 2. Create the site

1. Connect the git provider in the dashboard, then check it with `levelrail-cli git-providers`.
2. Create the app from the Git repository card in the **New** wizard, or from a spec file:

   ```yaml
   version: 1
   services:
     web:
       build:
         type: static
         baseDirectory: apps/web
         command: npm run build
         publish: dist
       domains: [www.example.com]
   ```

   ```bash
   levelrail-cli apps create --name my-site --file app.yaml --service web \
     --repo https://github.com/you/site.git --ref main
   ```

3. To change the build later, use the app's **Source** tab, or:

   ```bash
   levelrail-cli apps git-source set my-site --build-type static --build-command "pnpm build" --publish-dir build
   levelrail-cli apps git-source set my-site --build-type static --infer-build
   ```

   `--infer-build` detects the framework and fills whichever of the command and publish directory you leave out.

The build runs in a Node based container. It installs dependencies with the package manager that matches your lockfile, then runs your command. For Hugo, Jekyll or another generator, set `APP_STATIC_BUILD_IMAGE` to an image that has the tool and a shell, or build elsewhere and serve the committed output. The output is capped at `APP_STATIC_BUILD_MAX_MB` (default 2048). See [Static pages](/pages).

Leave `domains:` out of the first deploy so nothing competes with Netlify for the real hostname. Check the result with `levelrail-cli static-sites list`.

## 3. Map netlify.toml

Only the keys below map. Everything else in `netlify.toml` is ignored.

| netlify.toml | Levelrail | Notes |
| --- | --- | --- |
| `build.command` | `build.command` | Static services only |
| `build.publish` | `build.publish` | Relative to the base directory |
| `build.base` | `build.baseDirectory` | The project root inside the repo |
| `build.environment` | `build.args` | Build time variables for a static build. They end up in public output, so no secrets |
| `[context.deploy-preview]` | `contexts: deploy-preview` | Env, `build.command`, `build.publish`, `build.args` |
| `[context.branch-deploy]` | `contexts: branch-deploy` | Same fields |
| `[context.branch-name]` | `contexts: "branch:<pattern>"` | An exact name or a glob such as `release/*` |
| `[context.production]` | The app's own settings | Production takes no overrides |
| `[[redirects]]` | `routes:` in `app.yaml`, or `_redirects` | `from`, `to`, `status`, `force`, `query`. Conditions are not supported |
| `[[headers]]` | `headers:` in `app.yaml`, or `_headers` | `path`, `values`, `remove` |
| `[[redirects]]` with a proxy `200` to another site | Not supported | Use a path forwarder, see below |
| `build.ignore` | Not supported | Git source path filters are a different mechanism |
| `[functions]`, `[[edge_functions]]` | Not supported | See section 5 |
| `[[plugins]]` (build plugins) | Not supported | |
| `[dev]` | Not supported | |
| `[build.processing]` (asset optimization) | Not supported | |

### Redirects and headers

Copy `_redirects` and `_headers` into the published directory unchanged. They are compiled at deploy time and removed from the served files. Single page app fallback, clean URLs, trailing slash and the 404 page are set under `static:` in `app.yaml`:

```yaml
static:
  spa: true
  cleanUrls: true
  trailingSlash: remove
  notFound: 404.html
```

Check the files before you deploy, and test a URL against the live rules:

```bash
levelrail-cli static-sites routes lint my-site --redirects ./public/_redirects --headers ./public/_headers
levelrail-cli static-sites routes test my-site /old --request-query "ref=x"
```

Differences to expect:

- A rule that is not forced is skipped when a file exists at that path. Add `!` (or `force: true`) where you relied on a redirect winning over a file.
- `Country=`, `Language=`, `Role=` and `Cookie=` conditions are skipped and reported. These need an edge runtime.
- A `200` rewrite must point at a path on the same site. Proxying to another origin is not available in static routing. A [path forwarder](/domain-traffic-controls#path-forwarders) on the domain can send a path to an external URL or another app.
- Limits apply: 1000 rules, 100 header blocks and 256 KiB per file by default. See [Static routing](/static-routing).

## 4. Settings around the site

### Environment variables and deploy contexts

Static build env goes in `build.args`. For different values on previews and branches, set a context:

```bash
levelrail-cli apps contexts set my-site --context deploy-preview --env API_URL=https://staging.example.com
levelrail-cli apps contexts set my-site --context "branch:release/*" --build-command "npm run build:rc" --publish dist
levelrail-cli apps contexts show my-site --context branch-deploy --branch release/1.2
```

Context env is stored as plain values. Put secrets in branch env overrides (`apps branch-env set ... --secret`) or app secrets. Production deploys use the app's own settings only. See [Deploy contexts](/deploy-contexts).

### Branch deploys

Off by default. Turn them on for the branches you want:

```bash
levelrail-cli apps branch-deploys set my-site --mode patterns --branch staging --branch "feature/*"
```

Each branch gets a stable URL and rebuilds on every push. Removal happens when the branch is deleted or after `APP_BRANCH_DEPLOY_TTL` (default 336h). At most 10 live branch deploys per app by default.

### Deploy previews

Pull request previews are a separate feature, enabled per app with `levelrail-cli apps previews enable my-site`. They need a pull request and a primary domain on the control plane. See [Preview environments](/preview-environments). The docs do not say whether pull request previews cover static sites specifically, so try one on a test pull request before relying on it. Branch deploys are documented for static sites.

### Forms

Netlify finds forms in your HTML. Levelrail does not. Create each form and paste its snippet:

```bash
levelrail-cli forms create my-site contact --redirect-url https://example.com/thanks
levelrail-cli forms snippet my-site contact
```

The endpoint takes URL encoded, multipart and JSON posts. The `bot-field` input is the default spam trap, so a honeypot named `bot-field` can stay. Submissions have an inbox, search, CSV export and notifications to your existing alert channels. File uploads are not stored, there is no CAPTCHA check, and no confirmation goes back to the visitor. Forms are beta. Export old submissions from Netlify yourself, since there is no importer. See [Forms](/forms).

### Build hooks

```bash
levelrail-cli apps build-hooks create my-site --name "cms publish" --branch main
```

The URL is shown once. Point your CMS webhook at it. A POST answers `202` and builds the latest commit on the branch. Triggers are limited to 6 per hook per minute by default. See [Build hooks](/build-hooks).

### Scheduled functions

A Netlify scheduled function runs on a clock. Here the clock calls a path on a container app, and wakes it if it is asleep. A static site has no server to call, so move the handler to a [Functions](/functions) app or another container app, then:

```bash
levelrail-cli jobs create web digest --mode http --http-path /cron/digest --schedule "@hourly" --retries 3
```

Add `--background` for work that outlives a request. Http jobs cannot be declared in `app.yaml` yet, and the call carries no custom headers. See [Scheduled functions](/scheduled-functions).

### Lock deploys

Netlify's lock publishing maps to the publish lock. Builds still run, then wait as held builds until you publish them. Manual deploys and rollbacks still go live, as do safety rollbacks. Use the lock control on the app page, `levelrail-cli apps lock <app>` and `levelrail-cli apps unlock <app>`, or `PUT` and `DELETE` on `/api/v1/apps/{name}/publish-lock`. Publish a held build with `levelrail-cli apps publish <app> <deploy-id>`. See [Publish lock](/publish-lock).

### Split testing

Not available for static sites yet. Split testing works for container apps with a domain, between a branch deploy or a retained release and production. If you used Netlify split tests on a static site, there is no replacement today. See [Split testing](/split-testing).

### Drop deploy

For a site you build by hand, with no git:

```bash
levelrail-cli deploy ./site --static
levelrail-cli deploy site.zip
```

No build runs. What you drop is what is served. See [Drop deploy](/drop-deploy).

## 5. Where Netlify is ahead, and what has no equivalent

| Netlify feature | On Levelrail |
| --- | --- |
| Netlify Functions | [Functions](/functions) are container apps that sleep when idle, with a cold start of a container start. Rewrite the handler as a small HTTP server. Not an edge runtime |
| Background functions | An http job with `--background`, which the control plane waits on for up to 15 minutes by default |
| Edge Functions | Not offered. A stated non-goal. Logic that depends on the edge runtime has to move into your app or a CDN |
| Identity | No equivalent. Use a separate auth service or build login into an app. See [Integrations](/integrations) |
| Blobs | No equivalent. Managed Postgres and Redis exist, and S3-compatible storage is used for backups only |
| Image CDN | Not offered. Pre-generate sizes at build time |
| Analytics | No browser-side analytics. Server-side request rate, errors and latency percentiles per app are built in |
| Global CDN and edge network | Not provided. Files are served from the node or nodes you run |
| Automatic form detection | Not supported. Forms are created explicitly |
| Static split testing | Not supported yet |
| Redirect conditions (country, language, role) | Not supported |
| Proxy rewrites to external URLs in `_redirects` | Not supported there. Use a path forwarder |
| Build plugins | Not supported |

## 6. Domains and DNS cutover

1. Lower TTLs first. At least one old TTL period before the cutover, set the TTL on the relevant records to 60 to 300 seconds.
2. If the domain uses Netlify DNS, recreate the zone at a DNS host you control before you move anything, and copy every record, not only the site's.
3. Add the domains: `levelrail-cli domains add my-site example.com www.example.com`. Set the redirect between them with `levelrail-cli domains redirect set my-site www.example.com --target https://example.com`.
4. Get a certificate ready. HTTPS is self-signed until Let's Encrypt is enabled, and issuance needs the domain to already point at the server. See [Domains and ingress](/domains-and-ingress#tls-whats-actually-shipped-today).
5. Test before DNS changes: `curl --resolve example.com:443:<server-ip> https://example.com/` (add `-k` while the certificate is self-signed).
6. Cut over. Point the A and AAAA records at the Levelrail server. Keep the site on Netlify untouched.
7. Watch `dig +short example.com` from outside your network and the app's request metrics.
8. To roll back, restore the old records. Write the old values down first. A bad release on Levelrail is a different case: `levelrail-cli apps deploys rollback-to my-site DEPLOY_ID`, or the Rollback button on the deploy.

Raise TTLs again only after a clean run.

## Checklist

- [ ] Build command, publish directory and base directory match Netlify, and the site builds.
- [ ] `_redirects` and `_headers` pass `routes lint`, and key URLs behave the same under `routes test`.
- [ ] SPA fallback, clean URLs and the 404 page behave as before.
- [ ] Env vars are set, with preview and branch values in contexts.
- [ ] Branch deploys are on for the branches you use.
- [ ] Every form exists, its snippet is in the HTML, and a test post reaches the inbox and a notification channel.
- [ ] Build hooks are recreated and the CMS points at the new URLs.
- [ ] Scheduled work runs against a container app, and each job ran once.
- [ ] Every use of Functions, Edge Functions, Identity, Blobs, Image CDN and Analytics has an answer.
- [ ] Domains are added, the certificate is ready, and the TTLs were lowered.
- [ ] A rollback to the previous deploy works.

Decommission the Netlify site only after this passes.

## See also

- [Netlify alternative](/netlify-alternative)
- [Static pages](/pages)
- [Static site routing rules](/static-routing)
- [Domains and ingress](/domains-and-ingress)
- [Migrating from Vercel](/migrating-from-vercel)
