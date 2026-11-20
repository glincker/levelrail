---
title: "Migrating from Render"
description: "A runbook for moving Render services to Levelrail: the import command, concept mapping, environment variables, disks, databases, cron jobs, a DNS cutover and what has no equivalent."
"og:url": "https://docs.levelrail.com/migrating-from-render"
---

This is a runbook for moving Render services onto a Levelrail instance you run yourself. It covers only features that exist in this repository today. Where something has no equivalent, it says so.

<Warning>
The Render importer has been exercised against a fake Render API in tests, not against a real Render account. Always start with `--dry-run`, and keep the Render services running as your rollback path until the new deployment has run clean for a while. See [feature status](/feature-status) for the current maturity of each area.
</Warning>

## 1. Inventory what Render holds

Collect this before you touch Levelrail. Some of it is only in the Render dashboard.

- Every service, its type (web service, static site, private service, background worker, cron job) and its region and plan.
- Build command, start command, Docker command, Dockerfile path and root directory.
- Environment variables, and any Environment Groups they come from.
- Custom domains, your DNS provider and the current TTLs.
- Persistent disks: mount path and size.
- Render Postgres and Key Value instances: version, plan and disk size.
- Pre-deploy commands, auto-deploy settings, preview environments and autoscaling rules.
- A `render.yaml` Blueprint, if you have one. There is no Blueprint importer, so use it as a checklist.

## 2. Run the importer

`levelrail-cli import platform render` reads the Render API read-only and creates matching resources here. Create a Render API key, then pass it through the environment so it stays out of shell history:

```bash
export APP_IMPORT_SOURCE_TOKEN=...   # a Render API key, read only use
levelrail-cli import platform render --dry-run
```

The dry run prints a report of what would be created and what needs manual work, and creates nothing. The source URL defaults to the public Render API. When the report looks right:

```bash
levelrail-cli import platform render --only my-web,my-api --db-version postgres=16
levelrail-cli import platform render
```

Useful flags: `--only` limits the import to the named services, `--collision suffix|skip` decides what happens when a name is taken, `--db-version` sets an engine version when Render reports none. Re-running skips what was already imported. The token is sent to your control plane in the request body only and is never stored. See [Migrate to Levelrail](/migrate-to-levelrail) for how the other platform importers behave.

To review the result in a pull request before anything runs, write documents instead:

```bash
levelrail-cli import platform render --emit stack --out infra/stacks/shop \
  --stack shop --image-repo ghcr.io/acme
levelrail-cli stack plan shop -f infra/stacks/shop
```

The directory holds the stack documents, a `SECRETS.md` with secret names only and a `MIGRATION.md` checklist. See [Stacks](/stacks).

## 3. Concept mapping

| Render | Levelrail | Notes |
| --- | --- | --- |
| Web service from a git repository | App built from git | The importer records the repository, branch and root directory. Check the build type after import |
| Web service from an image | App from an image | The image path is carried over |
| Docker runtime | Dockerfile build | The Dockerfile path becomes the build path |
| Static site | App with a `static` build | Build command and publish directory are reported, set them after import. See [Static pages](/pages) |
| Private service | App with no public domain | Reported in the plan, keep it off the public routes |
| Background worker | App with no public route | Reported in the plan, set the same start command |
| Cron job | A scheduled job on an app | Reported as a warning, not created. See section 5 |
| Render Postgres | Managed Postgres | Created empty. Data is copied separately, see section 6 |
| Render Key Value | Managed Redis | Created empty. Pick the Redis version and size |
| Persistent disk | Volume | Mount path and size are carried over, the data is not. See section 6 |
| Environment variables | App env and secrets | Names and plain values are read. Secret handling is in section 4 |
| Environment Groups | [Shared env](/environment-and-secrets) | Not read by the importer, recreate them by hand |
| Custom domains | App domains | `onrender.com` subdomains are skipped |
| Health check path | Readiness path | Carried over |
| Instance count | Replicas | Carried over |
| Plan (CPU and memory) | Memory and CPU size | Reported, pick a matching size after import |
| Pre-deploy command | `hooks.preDeploy` in the app spec | Not read by the importer. See the [app spec reference](/app-spec-reference) |
| Auto-deploy on push | Git webhook on the app | Reported, connect the repository webhook after import |
| Preview environments | [Preview environments](/preview-environments) | Enabled per app, set up by hand |
| Autoscaling | [Autoscaling](/autoscaling) (beta) | Not read by the importer. Set a policy by hand |
| Blueprint (`render.yaml`) | App spec or a stack | No Blueprint importer. Use the file as a checklist |

## 4. Environment variables and secrets

The importer reads each service's variables. Keys that look like secrets are treated as secrets, so confirm the split in the dry run report. Variables Render generated for you (`generateValue` in a Blueprint) cannot be read back from Render: the report lists them and you set a new value yourself.

To adjust afterwards, or to load variables from a file:

```bash
levelrail-cli apps env import my-web --file .env --dry-run
levelrail-cli apps secrets set my-web --env-file secrets.env
```

The `--env-file` form keeps secret values out of your shell history.

Connection variables such as `DATABASE_URL` still point at Render until you change them. After a database exists here, `levelrail-cli databases link DB APP` injects the new connection variable. Never commit a `.env` file with real values.

## 5. Build, start commands and cron jobs

The importer reports the build command, start command and root directory it found, and does not apply every one of them. Set them on the app:

```bash
levelrail-cli apps git-source set my-web --build-type dockerfile --dockerfile Dockerfile --base-directory services/web
levelrail-cli apps git-source set my-web --build-type railpack
```

Apply runs prebuilt images. A service that builds from source needs its image built and pushed first, or a build created from the repository. When you emit a stack, `--image-repo` names the registry path.

A Render cron job becomes a scheduled job on an app that holds the same code. The importer prints the command and schedule it found:

```bash
levelrail-cli jobs create my-web nightly-report --schedule "0 3 * * *" -- python report.py
levelrail-cli jobs next --schedule "0 3 * * *" --count 3
```

Check the schedule with `jobs next`, then run it once with `jobs run my-web nightly-report`. See [Scheduled jobs](/scheduled-jobs).

## 6. Databases and disks

The importer creates databases and volumes empty. Move the data yourself, with writes stopped on the Render side so nothing is lost.

For Postgres, dump from Render with `pg_dump` using the connection string from the Render dashboard, then restore here:

```bash
pg_dump --format=custom --no-owner "$RENDER_DATABASE_URL" > shop.dump
levelrail-cli databases import shop-db --in shop.dump
levelrail-cli databases link shop-db my-web
```

Check the engine version first. Restore into a database of the same or a newer major version. See [Backups and storage](/backups-and-storage) for taking a backup before you cut over.

For a persistent disk, copy the files from the running Render service into a tar.gz archive (a shell session or one-off job on Render works), then fill the volume of the stopped app:

```bash
levelrail-cli apps volumes import my-web data --in disk.tar.gz
```

## 7. Domains and DNS cutover

1. Lower TTLs first. At least one old TTL period before the cutover, set the TTL on the affected records to 60 to 300 seconds.
2. Add the domains here: `levelrail-cli domains add my-web example.com www.example.com`. The command prints the exact DNS records to create.
3. Get a certificate ready. HTTPS is self-signed until Let's Encrypt is enabled, and issuance needs the domain to already point at the server. See [Domains and ingress](/domains-and-ingress#tls-whats-actually-shipped-today).
4. Test before DNS changes: `curl --resolve example.com:443:<server-ip> https://example.com/` (add `-k` while the certificate is self-signed).
5. Cut over. Point the A and AAAA records at the Levelrail server. Leave the Render service untouched.
6. Watch `dig +short example.com` from outside your network and the app's request metrics.
7. To roll back, restore the old records. Write the old values down first.

Raise TTLs again only after a clean run.

## 8. Limits and gaps

- The importer does not migrate data: database contents and disk files are copied by hand.
- Render cron jobs, Environment Groups, pre-deploy commands, autoscaling rules, preview environment settings and Blueprints are not created by the importer.
- Render private networking between services does not carry over. Check how your apps reach each other here, see [Private networks](/private-networks).
- Region and plan names do not map. Choose nodes and sizes yourself.
- Render's global edge and DDoS layer are not part of Levelrail. Files and traffic are served from the nodes you run.
- Anything the report marks as unsupported needs a manual step. The report names each one.

## Checklist

- [ ] The dry run report was read, and every manual item has an owner.
- [ ] Every app builds or pulls its image and passes its health check.
- [ ] Env vars and secrets are set, and none still points at a Render hostname.
- [ ] Databases are restored, row counts match, and apps are linked to them.
- [ ] Disk contents are restored into the volumes.
- [ ] Cron jobs exist and each ran once.
- [ ] Domains are added, the certificate is ready and the TTLs were lowered.
- [ ] A rollback to the previous deploy works: `levelrail-cli apps deploys rollback-to my-web DEPLOY_ID`.

Decommission the Render services only after this passes.

## See also

- [Migrate to Levelrail](/migrate-to-levelrail)
- [Migrating from Vercel](/migrating-from-vercel)
- [Migrating from Netlify](/migrating-from-netlify)
- [Stacks](/stacks)
- [Domains and ingress](/domains-and-ingress)
