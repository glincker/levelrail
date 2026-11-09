---
title: "Coolify exit gap report"
sidebarTitle: "Coolify exit gaps"
description: What the Coolify importer carries over, what it reports for you to finish, and what it cannot see yet, audited against what a real Coolify instance holds.
"og:url": "https://docs.levelrail.com/coolify-exit-gap-report"
---

This is the audit behind the [Coolify exit plan](/migrate-from-coolify#exit-report-and-cutover-checklist). It compares what a Coolify v4 instance holds with what the importer reads, and says for each item whether it moves, is reported, or is invisible to the tool.

## How far to trust this

No real Coolify instance was available while this was written. The importer was checked against JSON fixtures shaped from Coolify's documented API responses, with fake credentials. That proves the mapping logic, the read-only guarantee (the fixture server rejects every non-GET) and the report. It does not prove that a field name or route matches your Coolify version. Field names such as `is_build_time`, `manual_webhook_secret_github` and the `/databases/{uuid}/backups` route are best knowledge, and a read that fails softly shows up as a note on the report instead of stopping it.

Before you delete Coolify, run `migrate coolify --dry-run` against your own instance, then compare the output with the Coolify dashboard for two or three apps you know well.

## Audit

Status words: **moves** means created on the new side, **reported** means the tool names it and you finish it, **invisible** means the tool does not read it at all.

| Coolify holds | Status | Notes |
| --- | --- | --- |
| Applications from git (Dockerfile, Nixpacks, static) | moves | Created stopped. Nixpacks becomes Railpack and is flagged because the build may differ. Connecting a private repo is a manual step |
| Applications from a registry image | moves | Image and tag kept |
| One-click services | reported | Compose body and env are read. The file and `--apply` modes of `migrate coolify` stage each one as `compose.yaml` under `--stage-dir`, with `SERVICE_` magic variables translated. Nothing is deployed. Dry-run and verify list it as skipped. See [one-click services to Compose](/coolify-service-to-compose). Pending run against a real instance |
| Docker Compose applications (`dockercompose` build pack) | staged | `migrate coolify` stages `docker_compose_raw` as one `compose.yaml` under `--stage-dir`, with `SERVICE_` magic variables and generated secrets resolved. The file goes through the same host-access rules as a manual compose deploy: `privileged`, host network, host `pid`/`ipc`, `cap_add`, `devices` and Docker socket mounts keep the app blocked with the reason. Deploy it with [compose apps](/compose-apps). Pending run against a real instance |
| Environment variables and secrets | moves | Secret-looking names go through the encrypted store. Preview duplicates are dropped. Secrets the token could not read are listed by name |
| Build-time only flag on variables | moves | `migrate coolify --apply` stores them as secrets with build access, values never printed. The platform import keeps them as runtime variables and lists the names |
| Install, build and start commands | moves | `migrate coolify` writes a static build as `build.command` (install and build joined) with `build.publish`, and a start command as `command:`. Railpack and Dockerfile builds list install and build commands as planned items. A command over `APP_IMPORT_COMMAND_MAX` bytes, spanning lines or setting a secret-looking value inline is not carried, and its text is not printed |
| Pre and post deployment commands | moves | Become deploy hooks (`hooks.preDeploy`, `hooks.postDeploy`). A static site has no container, so they are reported |
| Publish directory of a static site | moves | Becomes `build.publish`, or `build.path` when there is no build command |
| Base directory and Dockerfile path | moves | Become `build.baseDirectory` and `build.path`. An absolute or `../` path is rejected and reported, never rewritten |
| Node version pin (`NODE_VERSION`, `NIXPACKS_NODE_VERSION`) | reported | Planned item: pin Node in the repository (`engines.node` or `.node-version`) so Railpack reads it |
| Tags | reported | Planned item, there is no tag field here |
| Domains | moves | Read, then held back until cutover. A `www` or `non-www` redirect rule becomes the matching canonical redirect on the first domain (`--apply`) |
| TLS certificates | moves | Issued on first use after DNS points here. Custom uploaded certificates are not exported by Coolify's API |
| HTTP basic auth on an app | translated | The app is public until basic auth is set, and the report says so in both modes. `--apply` gates every domain with basic auth, user `admin` and a random placeholder password that is never printed, logged or written to a report. The source credential is never read. Set the real one with `domains basic-auth set` |
| Custom container labels (often Traefik middleware) | translated | `redirectscheme` to https becomes force HTTPS, `redirectregex` becomes a redirect forwarder rule, `stripprefix` a strip-prefix forwarder rule, `headers` custom request and response headers become header rules, `basicauth` turns on the gate above. Every rule passes the same validators as a domain policy edit: http and https targets only, length caps from `APP_IMPORT_MAX_LABEL_BYTES`, `APP_IMPORT_MAX_REDIRECT_URL_LEN` and `APP_DOMAIN_POLICY_MAX_REGEX_LEN`, a regex size cap from `APP_IMPORT_MAX_REGEX_INSTS`, no line breaks in header values. A redirect regex scoped to a host other than the app's first domain, a replacement with a query, fragment or user part, a header with a secret-looking name or a token-like value (`APP_IMPORT_OPAQUE_VALUE_MIN_LEN`), a replacement with capture groups, and any other label key stay as manual items with the reason, the value is never shown |
| Custom docker run options | reported | Capabilities, devices, ulimits |
| Persistent volumes | moves | Created empty. The data copy is a separate step with file and byte counts |
| File mounts | copied | Each file storage becomes an app file at its mount path with identical bytes (`apps files set`). A path that is not a clean absolute file path (relative, `..`, `.`, backslash, control character, reserved root) is refused with the reason, and content over `APP_SERVICE_FILE_MAX_BYTES` or not text is refused. Content is copied only with `--include-secret-values` (otherwise the report lists path and size), and a secret-looking name or content goes through the encrypted secret store. Duplicate and nested mount paths are reported. Directory mounts are reported |
| Databases: PostgreSQL, MySQL, MariaDB, MongoDB, Redis | moves | Created empty at the same major version. Data copy uses `migrate db-copy`, and the report prints the command with the public port, user and database name read from Coolify |
| Databases: KeyDB, Dragonfly, ClickHouse | moves | Created empty as managed databases. Dragonfly keeps a full `vX.Y.Z` tag and KeyDB keeps an exact `eqalpha/keydb` release tag (such as `x86_64_v6.3.4`), other tags fall back to the newest supported version. The report prints the `migrate db-copy` command for each |
| Database backup schedules | reported | One row per schedule with the `backups schedule set` command. Imported databases also get a manual row for their schedule |
| Scheduled tasks (cron) | moves | Was report-only. Enabled tasks become scheduled jobs that run `sh -c COMMAND` inside the app. Disabled tasks are skipped. Staged apps get their jobs disabled until cutover so a job never runs on both hosts |
| Git webhooks (auto deploy) | moves, opt-in | With `--create-webhooks` and `--apply`, one push webhook per git-sourced app is registered through the connected git provider whose https scheme, host and port match the repository. Off by default. The source secret is never read, the control plane generates a new one and never prints it. Re-runs do not duplicate hooks, skipped apps get the reason, and the dry run lists what would be created |
| HTTP health checks | moves | Path, interval, timeout and retries |
| Command health checks | moves | Built, unit tested, source field name not verified against a real instance. A command check becomes an `exec` probe (`sh -c COMMAND`, see the [app spec](/app-spec-reference)) with interval, timeout and retries. A missing, oversized or unrecognised command keeps a note to add the probe by hand |
| Memory and CPU limits | moves | Memory and CPU are mapped. A swap or memory reservation set on the source has no setting here, so it is listed as a note on the app with its value |
| Preview deployments | translated | Previews are turned on with the default preview policy on `--apply`. When the source used basic auth, previews get the preview gate with a random placeholder password, so preview URLs stay private. Review the policy before the first pull request |
| Projects and environments | moves | Each Coolify project becomes a project and each of its environments (empty ones too) becomes an environment of it. Apps and databases keep their own environment. The dry run lists every environment, and a re-run skips the ones that exist |
| Servers and destinations | reported | New. The report lists every server, the proxy that holds ports 80 and 443, and warns when there is more than one deploy server. With `--node-map source-server=node` each app goes on the named node (`apps set-node`). An unmapped server keeps the app on the control plane node with a finding, and an unknown or ambiguous node name, or a token without the root ability, is refused before anything is created |
| Docker networks | reported | An app on a custom network shared with another imported app, database or service gets a note naming the others. No connection is created automatically, so a name that happens to match an unrelated database never links to it. Use the service name or internal host as the address. Read from the data already fetched, no extra request. Networks the API does not expose are not seen |
| Team members, roles, API tokens | invisible | Not migrated by design. Recreate users and tokens here |
| S3 storage destinations | reported | One row per destination with a `backup-targets create` command. Keys and bucket secrets are never read, so you enter them again |
| SSH keys, GitHub App credentials | invisible | Credentials are never read. Add them again |
| Notification channels (Discord, Slack, email) | invisible | Recreate them as [alert channels](/alert-inbox) |
| Deploy history, logs, metrics | invisible | Not migrated |

## What the exit plan adds

- `migrate coolify --dry-run` prints a precise plan, one row per resource with its outcome: **imported**, **manual** or **skipped**, each with the reason and the next step.
- The same report is available over the API, in the dashboard import flow and as a read-only MCP tool.
- A DNS switch order (stateless apps first, apps with a volume and a database last) and an ordered cutover checklist that keeps Coolify as the fallback until you have verified.
- `ready_to_retire` is true only when every resource is imported and nothing needs a person. Skipped items always block it.

## Not proven yet

- The environment create step in the guided import API (the applier now creates environments and places apps and databases in them, covered by a unit test but not yet run against a real Coolify instance).
- A run against a real Coolify instance, including large instances with many environments.
- That Coolify's API returns the fields above for every build pack and version.
- Restoring a backup copied across, and the write pause needed for a database that takes writes during the final copy.
- Fallback to Coolify after the first write lands on the new database. Copy the data back first.
- The `destination.network` and memory swap field names in the application payload, which are assumed from the documented API and not checked on a real instance.
- One-click service staging (compose translation, generated secrets and the `compose deploy` step) against a real Coolify instance. Pending run.

## Checked against the code

Last checked against the importer in `internal/platformimport/coolify.go` on 2026-11-08. Backup schedules and S3 destinations are still only listed as manual rows with the command to run (`reportBackups`); nothing creates them during `--apply` yet. Every row marked built or moves is covered by fixture tests in `internal/platformimport` and `cmd/levelrail-cli`, none by a real instance.

## Next gaps, by value

1. Import database backup schedules and S3 targets instead of listing them.
2. Run `--create-webhooks` against a real Coolify instance and real GitHub, GitLab, Gitea and Bitbucket connections. It is covered by fake-provider tests only.
