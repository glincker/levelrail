# Migrating from Coolify, Dokploy, or CapRover

`levelrail-cli migrate coolify`, `migrate dokploy`, and `migrate
caprover` read every application off a live source instance and turn
each one into a Levelrail service, either as `app.yaml` files on disk
or applied directly to a target Levelrail control plane. This is a
one-way, read-only migration: nothing on the source instance is
touched, and applying to Levelrail is additive (it creates new apps,
it doesn't delete or modify anything on the source).

This page describes exactly what the command does today, not an
aspirational version of it. If something below says an app is dropped
or needs manual review, that's the actual behavior of the mapping
code, not a gap in this doc.

## Before you run it

You need:

- The source instance's base URL (`--url`) and a credential
  (`--token`): a Coolify API bearer token, a Dokploy API key, or (for
  CapRover) the instance's own login password. Coolify and Dokploy pass
  `--token` straight through as a bearer credential; CapRover is
  different, it authenticates via a login exchange
  (`CaproverClient.Login`), so `--token` there is the plain login
  password, not a pre-issued API token, exchanged for a session token
  automatically before anything else happens.
- A decision on `--include-secret-values`. Without it, every
  environment variable is migrated as a key-only placeholder (the
  variable name carries over, the value doesn't), and the report tells
  you how many were skipped this way per app. With it:
  - Coolify additionally requires the token to have the
    `read:sensitive` ability; if a value still comes back empty even
    with the flag set, that's reported as a review issue naming the
    specific keys, not silently dropped.
  - Dokploy has no separate read-sensitive gate: its API already
    returns real values in the app's env blob, so `--include-secret-values`
    only controls whether this tool writes or applies them.
  - CapRover has no documented redaction gate either, the same trust
    model as Dokploy: real values come back in the same listing this
    tool already reads, so `--include-secret-values` only controls
    whether they're written or applied.
- A choice between file mode (default) and `--apply`:
  - File mode writes one `<service>.yaml` per successfully mapped app
    into `--out-dir` (default `./migrated`), plus a `<service>.secrets.env`
    file (mode 0600) for any app whose real env values were fetched.
    That secrets file is plain `KEY=VALUE` and is never referenced by
    the generated `app.yaml`; it's meant to be read once (to run
    `apps secrets set`, or paste into a dashboard's Secrets card) and
    deleted.
  - `--apply` calls `POST /api/v1/apps` directly against a target
    Levelrail instance for every successfully mapped app, then sets
    each fetched secret value via `PUT .../secrets/{key}`. It needs
    its own credentials: `--target-token`, `--target-api-url`, and
    `--target-profile` are separate from `--token`/`--url` above
    because this is the one CLI command that talks to two different
    control planes in one invocation. They resolve through the same
    `APP_API_TOKEN`/`APP_API_URL`/named-profile precedence every other
    command uses.

## What actually gets migrated

All three commands map to the same `mappedApp`/`mappedService` shape,
so they behave identically once you're past provider-specific fetch
calls (Coolify: application detail + env vars; Dokploy: application
detail + domains; CapRover: the app definitions listing, which already
carries domains and env vars inline).

Mapped automatically, per app:

- Build type: `dockerfile`, `railpack`, or `static` for Coolify and
  Dokploy (their `build_pack`/`buildType` fields both map onto
  Levelrail's own three; a non-default Dockerfile path is preserved as
  `build.path`). CapRover is different: its app-definitions listing
  doesn't expose the `captain-definition` file's contents (a Dockerfile
  path, a template ID, or inline Dockerfile lines all look the same
  from this API), so every CapRover app's build type is **assumed** to
  be `dockerfile` at the repo root and flagged as a `review` issue
  naming the actual `captainDefinitionRelativeFilePath`, confirm it
  before deploying.
- Domains: Coolify's comma-separated `fqdn` field, Dokploy's attached
  domain list, or (for CapRover) the app's default subdomain
  (`<appName>.<rootDomain>`, unless the app opted out of web exposure)
  combined with every explicit custom domain, all folded into the same
  domains list.
- Port: the first port found (Coolify's `ports_exposes`, Dokploy's
  first domain's port, CapRover's `containerHttpPort`). A static site
  gets no port, since it has no running container to route to.
- HTTP health checks: Coolify's path-based health check config maps
  directly (interval, timeout, retry count). Dokploy and CapRover have
  no documented equivalent field either source's API exposes today, so
  apps from either don't get one migrated; add it by hand afterward.
- Resource limits: Coolify's Docker-native `limits_memory`/`limits_cpus`
  strings and Dokploy's byte/nanoCPU fields both convert into
  `resources.memory`/`resources.cpu`. CapRover exposes no per-app
  resource limits at all in this listing, so nothing is mapped or
  flagged, it's simply absent.
- Environment variable keys: always migrated, for all three sources.
  Real values: only with `--include-secret-values`, and only into the
  secrets file or applied via the secrets API, never written into the
  generated `app.yaml`.
- Git repo and branch, where the source API exposes an unambiguous
  host: Coolify's own `git_repository`/`git_branch` fields directly;
  for Dokploy, GitHub and Bitbucket sources (owner + repository +
  branch, built into a `https://github.com/...` / `https://bitbucket.org/...`
  URL) and a custom git URL source. Dokploy's GitLab and Gitea source
  types are **not** turned into a repo URL, because Dokploy's API
  returns only `owner`/`repository`/`branch` for those, not the
  instance host (both are commonly self-hosted), and guessing a host
  would be worse than leaving it blank. Those come back as a review
  issue with the owner/repository/branch values so you can build the
  URL yourself. **CapRover exposes no git repo/branch identity at all**
  on this endpoint (a CapRover deploy is a one-time push, not stored
  state), so a CapRover-sourced `app.yaml` never has one and this tool
  never prints the follow-up `apps create --repo ...` command for it,
  unlike Coolify and Dokploy.

Reported as needing manual review or dropped (per app, in the
`issues` list, each tagged `dropped`, `review`, or `blocking`):

- Coolify's `nixpacks` build pack and Dokploy's `nixpacks` build type:
  **blocking**. Levelrail uses Railpack, not Nixpacks, and this tool
  does not attempt to translate between them.
- Coolify's `dockercompose` build pack: **blocking**. Compose apps are
  multi-service by nature and this tool doesn't attempt to translate a
  compose file into Levelrail's `services:` map.
- Dokploy's `docker` (prebuilt image) and `drop` (file upload) source
  types: **blocking**. `app.yaml` has no field for a bare image today
  outside of the CLI's own `--image-repo` create flow, so these need a
  manual `apps create`.
- Dokploy's `heroku_buildpacks`/`paketo_buildpacks` build types:
  **blocking**, same reason as nixpacks.
- Coolify's command-based (`CMD`) health checks: **dropped**. Levelrail
  only supports HTTP-path health checks.
- A second or later port on a multi-port app: **dropped**, since
  `app.yaml` supports one port per service. CapRover's raw host<->container
  TCP/UDP port mappings are **dropped** the same way: `app.yaml` only
  has room for the one ingress-routed port every mapped app already
  gets above.
- CapRover persistent volumes and bind mounts: **dropped**, one issue
  per volume naming the container path, since `app.yaml` has no
  service-level volume field. If `hasPersistentData` is set but no
  volumes were listed, that's a separate **review** issue telling you
  to check the CapRover dashboard by hand.
- CapRover `instanceCount` above 1: **review**. This tool doesn't set
  `app.yaml`'s `replicas` field for any of the three sources (none of
  their app models expose a count this tool maps), so a scaled-out
  CapRover app is called out explicitly rather than silently deployed
  at Levelrail's single-replica default.
- An application name that isn't already a valid Levelrail service
  name (lowercase alphanumeric and hyphens, starting with a letter):
  **review**. It's sanitized automatically and the original name is
  recorded in the issue.
- Any resource-limit value this tool can't parse: **review**, with the
  limit left unset on the Levelrail side rather than guessed.

An app that hits a **blocking** issue produces no `app.yaml` and is
listed separately in the report ("needs manual review (not
migrated)"); every other issue severity still produces a usable
`app.yaml`, with the caveat recorded alongside it.

## What never carries over, by design

- **Secret values**, unless you pass `--include-secret-values`, and
  even then they never land inside the generated `app.yaml` itself
  (see above). Re-entering secrets through `apps secrets set` or the
  dashboard's Secrets card is the intended path.
- **TLS certificates.** Levelrail's ingress issues its own (an
  internal self-signed issuer by default, a public ACME issuer as a
  settings toggle). Nothing about a source instance's existing
  certificates is read or migrated.
- **DNS.** Domains are mapped into the generated `app.yaml`, but the
  DNS records themselves still point at the old instance until you
  repoint them. Plan for a cutover window per domain, not an instant
  switch.
- **Deploy history, logs, and metrics.** The migration only reads
  current application configuration, not historical data.
- **Git source registration.** Even for apps where a repo URL and
  branch were recovered, Levelrail doesn't persist git/build
  configuration as part of `POST /api/v1/apps` (see the next section);
  the generated `app.yaml` records the intended source, but no build
  is triggered by the migration itself.

## The follow-up step every migrated app with a repo needs

Levelrail doesn't store git repo/branch identity as part of an app
resource; a build has to be explicitly triggered with a repo, ref, and
target image. For every migrated app that had a recoverable repo URL
(never the case for CapRover, see above), both the file-mode and
`--apply` report print the exact follow-up command:

```
levelrail-cli apps create --file ./migrated/<service>.yaml \
  --repo <repo-url> --ref <branch> --image-repo <your-registry>/<service>
```

In `--apply` mode the app itself already exists on the target
instance (created with a placeholder image and no build yet), so
adapt the command to point at your registry and run it once you're
ready to build; there's no file to reference if you didn't use file
mode; drop `--file` and confirm the app name matches what apply
already created.

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

Both commands support the same global CLI flags: `--output
json|table|text` (the human-readable summary above is the `text`
default; `--json` is a shorthand for `--output json` kept for backward
compatibility) and `--query` for a JMESPath filter over the result, so
the migration report itself is scriptable if you're migrating a large
number of apps and want to pipe the "blocking" list into another tool.
