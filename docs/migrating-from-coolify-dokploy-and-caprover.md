# Migrating from Coolify, Dokploy, or CapRover

The `migrate` commands read every application from a live source instance and convert each one into a Levelrail service.

Commands:

- `levelrail-cli migrate coolify`
- `levelrail-cli migrate dokploy`
- `levelrail-cli migrate caprover`

Output can be saved as `app.yaml` files on disk or applied directly to a target Levelrail control plane.

This is a one-way, read-only migration. Nothing on the source instance is touched, and applying to Levelrail is additive (creates new apps, does not delete or modify existing ones on the source).

::: tip
This page describes what the commands do today, not an aspirational version. If something below says an app is dropped or needs manual review, that's the actual behavior of the mapping code, not a gap in this documentation.
:::

## Before you run it

### Source credentials

You need the source instance's base URL (`--url`) and a credential (`--token`):

- **Coolify**: API bearer token
- **Dokploy**: API key
- **CapRover**: Instance login password (plain text, not a pre-issued API token)

Coolify and Dokploy pass `--token` straight through as a bearer credential. CapRover authenticates via login exchange (`CaproverClient.Login`), exchanging the password for a session token automatically.

### Secret values

Decide whether to include secret values with `--include-secret-values`.

Without it:
- Every environment variable migrates as a key-only placeholder (name only, value skipped).
- The report shows how many were skipped per app.

With it:

- **Coolify**: requires the token to have the `read:sensitive` ability. Empty values (even with the flag set) are reported as review issues, not silently dropped.
- **Dokploy**: no separate sensitive gate; API already returns real values. The flag only controls whether this tool writes or applies them.
- **CapRover**: no documented redaction gate; same trust model as Dokploy. The flag only controls whether values are written or applied.

### Output mode: file or direct apply

Choose file mode (default) or `--apply`:

**File mode**: Writes one `<service>.yaml` per app into `--out-dir` (default `./migrated`), plus a `<service>.secrets.env` file (mode 0600) for any app whose real env values were fetched. The secrets file is plain `KEY=VALUE` text, meant to be read once and deleted (use `apps secrets set` or the dashboard's Secrets card to enter the values).

**Apply mode** (`--apply`): Calls `POST /api/v1/apps` directly against a target Levelrail instance for each app, then sets each fetched secret via `PUT .../secrets/{key}`. Requires separate credentials: `--target-token`, `--target-api-url`, and `--target-profile` (separate from `--token`/`--url`) because this command talks to two control planes in one invocation. They resolve through the same `APP_API_TOKEN`/`APP_API_URL`/named-profile precedence as other commands.

## What actually gets migrated

All three commands map to the same `mappedApp`/`mappedService` shape, so they behave identically once past provider-specific fetch calls.

### Automatically migrated per app

**Build type**

- Coolify and Dokploy: `dockerfile`, `railpack`, or `static` (from their `build_pack`/`buildType` fields); non-default Dockerfile paths preserved as `build.path`.
- CapRover: **assumed** to be `dockerfile` at the repo root (API does not expose `captain-definition` file contents). Flagged as a `review` issue naming the actual `captainDefinitionRelativeFilePath` so you can confirm before deploying.

**Domains**

- Coolify: comma-separated `fqdn` field
- Dokploy: attached domain list
- CapRover: app's default subdomain (`<appName>.<rootDomain>`, unless the app opted out) combined with explicit custom domains

All folded into the same domains list.

**Port**

The first port found: Coolify's `ports_exposes`, Dokploy's first domain's port, or CapRover's `containerHttpPort`. Static sites get no port (no running container to route to).

**HTTP health checks**

- Coolify: path-based config maps directly (interval, timeout, retry count).
- Dokploy and CapRover: no documented equivalent field in the API, so health checks are not migrated. Add by hand afterward.

**Resource limits**

- Coolify: Docker-native `limits_memory`/`limits_cpus` strings convert to `resources.memory`/`resources.cpu`.
- Dokploy: byte/nanoCPU fields convert the same way.
- CapRover: exposes no per-app resource limits, so nothing is mapped.

**Environment variable keys**

Always migrated for all three sources. Real values only migrate with `--include-secret-values`, and only into the secrets file or secrets API, never into the generated `app.yaml`.

**Git repo and branch**

- Coolify: `git_repository`/`git_branch` fields migrate directly.
- Dokploy: GitHub and Bitbucket sources (owner + repository + branch) become URLs; GitLab and Gitea sources do not (API returns only `owner`/`repository`/`branch`, not the instance host, and guessing a host is worse than leaving it blank). These come back as a review issue with the values so you can build the URL yourself.
- CapRover: **exposes no git repo/branch identity** on this endpoint (CapRover deploys are one-time pushes, not stored state). CapRover-sourced `app.yaml` files have no repo/branch, and the follow-up `apps create --repo ...` command is not printed.

### Issues reported for manual review or dropped

Per app, in the `issues` list, each tagged `dropped`, `review`, or `blocking`:

#### Blocking issues (app not migrated)

- Coolify's `nixpacks` build pack, Dokploy's `nixpacks` build type: Levelrail uses Railpack, not Nixpacks; no translation is attempted.
- Coolify's `dockercompose` build pack: Compose apps are multi-service by nature; translation to Levelrail's `services:` map is not attempted.
- Dokploy's `docker` (prebuilt image) and `drop` (file upload) source types: `app.yaml` has no field for a bare image outside the CLI's `--image-repo` create flow; these need a manual `apps create`.
- Dokploy's `heroku_buildpacks` and `paketo_buildpacks` build types: Same reason as nixpacks.

Apps with blocking issues produce no `app.yaml` and are listed separately ("needs manual review (not migrated)").

#### Dropped issues (silently omitted)

- Coolify's command-based (`CMD`) health checks: Levelrail only supports HTTP-path health checks.
- Additional ports on multi-port apps: `app.yaml` supports one port per service. Later ports are dropped.
- CapRover's raw host/container TCP/UDP port mappings: Same reason; only the ingress-routed port is kept.
- CapRover persistent volumes and bind mounts: `app.yaml` has no service-level volume field. One issue per volume names the container path. If `hasPersistentData` is set but no volumes were listed, a separate **review** issue tells you to check the CapRover dashboard by hand.

#### Review issues (app migrated with caveats)

- CapRover `instanceCount` above 1: This tool doesn't set `app.yaml`'s `replicas` field for any source (no app model exposes a count this tool maps), so scaled-out CapRover apps are called out explicitly rather than silently deployed at Levelrail's single-replica default.
- Application name not valid for Levelrail (must be lowercase alphanumeric and hyphens, starting with a letter): Name is sanitized automatically; the original name is recorded in the issue.
- Any resource-limit value this tool can't parse: Limit is left unset on the Levelrail side rather than guessed.

Every review/dropped issue still produces a usable `app.yaml`, with the caveat recorded alongside it.

## What never carries over, by design

**Secret values**

Unless you pass `--include-secret-values`, secrets do not migrate. Even with the flag, they never land inside the generated `app.yaml`. Re-enter secrets through `apps secrets set` or the dashboard's Secrets card.

**TLS certificates**

Levelrail's ingress issues its own (internal self-signed by default, public ACME as a settings toggle). Existing certificates from the source instance are not read or migrated.

**DNS records**

Domains are mapped into the generated `app.yaml`, but DNS records still point at the old instance until you repoint them. Plan for a cutover window per domain, not an instant switch.

**Deploy history, logs, and metrics**

The migration reads only current application configuration, not historical data.

**Git source registration**

Even for apps where a repo URL and branch were recovered, Levelrail doesn't persist git/build configuration as part of `POST /api/v1/apps` (see the next section). The generated `app.yaml` records the intended source, but no build is triggered by the migration itself.

## The follow-up step every migrated app with a repo needs

Levelrail doesn't store git repo/branch identity as part of an app resource; a build must be explicitly triggered with a repo, ref, and target image.

For every migrated app that had a recoverable repo URL (never the case for CapRover), both file-mode and `--apply` reports print the exact follow-up command:

```
levelrail-cli apps create --file ./migrated/<service>.yaml \
  --repo <repo-url> --ref <branch> --image-repo <your-registry>/<service>
```

**In file mode**: Run this command once you're ready to build (the app doesn't exist on Levelrail yet).

**In `--apply` mode**: The app already exists on the target instance (created with a placeholder image, no build yet). Adapt the command to point at your registry, drop `--file`, confirm the app name matches what apply already created, then run it.

## Manual-check punch list

After running any of the three commands, before you trust the result:

- [ ] Every app in "needs manual review (not migrated)": build it by
      hand (a Compose app, a prebuilt-image app, a Nixpacks/buildpacks
      app).
- [ ] Every `review`-severity issue on a migrated app: read it, it
      names the exact field and why it needed a human decision. For
      CapRover specifically, that includes confirming the actual
      `captain-definition` build method (this tool always assumes
      `dockerfile` at the repo root, since the API doesn't expose it)
      and checking the CapRover dashboard directly for any app flagged
      as having persistent data with no volume recorded.
- [ ] Re-enter every secret value (`apps secrets set <app> <key>`, or
      the dashboard's Secrets card) unless you used
      `--include-secret-values`, in which case: apply the values, then
      delete the `.secrets.env` file, it's plain text on disk.
- [ ] Re-point DNS for every migrated domain once you've confirmed the
      app is healthy on Levelrail, and only then, so you keep a
      rollback path to the old instance.
- [ ] Trigger the actual build for every app with a repo, using the
      follow-up `apps create --repo ... --ref ... --image-repo ...`
      command the report printed. Every CapRover-sourced app needs this
      done manually from scratch instead, since none of them get a repo
      URL recovered at all.
- [ ] Re-create storage for every CapRover app that had a volume or
      bind mount: none of it carries over, and an app with
      `hasPersistentData` set but no volume recorded needs a trip to
      the CapRover dashboard to find out what's actually there.
- [ ] Set `replicas` by hand for any CapRover app whose `instanceCount`
      issue mentioned scaling beyond 1; every migrated app otherwise
      lands at Levelrail's single-replica default regardless of source.
- [ ] Re-check health checks: Dokploy- and CapRover-sourced apps had
      none migrated; Coolify command-based checks were dropped. Add an
      HTTP-path check in `app.yaml` or the dashboard's Health tab.
- [ ] Re-verify resource limits landed as expected; a limit this tool
      couldn't parse is left unset, not defaulted to something
      conservative. CapRover apps never had resource limits mapped in
      the first place, its listing doesn't expose any.
- [ ] Confirm every domain in the generated `app.yaml` before wiring
      up TLS. static sites and services with a domain-but-no-port
      (multi-port sources) are the cases most likely to need a second
      look.

## Output formats

Both commands support the same global CLI flags for scriptable output:

- `--output json|table|text` (the human-readable summary is the `text` default; `--json` is a backward-compatible shorthand for `--output json`)
- `--query` for JMESPath filtering

The migration report itself is scriptable, so you can pipe the "blocking" list into another tool when migrating a large number of apps.
