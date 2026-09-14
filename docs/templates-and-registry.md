# Service template catalog

Deploy a curated set of ready-made services (n8n, Uptime Kuma, Postgres-backed
apps, and so on) in one step, without hand-writing a Compose file. Package:
`internal/catalog/catalog.go`, `internal/api/service_templates.go`,
`cmd/levelrail-cli/templates.go`.

## Why this exists

Section 2's original non-goals list ruled out a large template catalog on
purpose: ten good templates beat 280 stale ones. That got reversed (ADR 015)
after a direct side-by-side comparison of this platform's own "New resource"
modal, five plain cards, against a competing self-hosting platform's
categorized, searchable catalog. The founder asked for the real thing, not a
UX-only overhaul of the existing five cards.

The catch: every template in a dataset like that is a Docker Compose file,
and the reconciler (`internal/reconcile/application`) has only ever managed
one container per service. So the actual sequencing was Compose support
first, template catalog second, both landing together. A template is not a
separate deploy mechanism bolted on top: it is a curated `compose.yaml` body
that gets handed to the exact same endpoint `apps deploy-compose` already
uses. There is no template-specific reconciler path, no template-specific
database table beyond the static catalog data itself.

## How it actually works

`internal/catalog/catalog.go` holds `Templates`, a hardcoded Go slice of
`Template` structs (`ID`, `Name`, `Slogan`, `Category`, `DocumentationURL`,
`Compose`). It is written fresh for this platform, not copied from any other
project's dataset, and it is served straight out of memory: no database
table, no admin UI to add entries, no version field. Adding a template today
means adding an entry to that Go slice and shipping a new control plane
binary.

`GET /api/v1/service-templates` returns the catalog without each entry's
`Compose` body, deliberately, so the browse grid's initial load stays small.
`GET /api/v1/service-templates/{id}` returns one entry including the full
Compose text. Deploying is a separate, explicit step: the caller (dashboard
or CLI) fetches an entry's Compose body, then POSTs it to
`POST /api/v1/apps/{name}/compose`, the same route `apps deploy-compose`
already calls with a hand-written file. A compose document fans out into one
`store.App` plus one `store.DesiredService` row per compose service
(`migrations/0039_apps.sql`'s app/service grouping), each still reconciled
independently by the existing single-container controller.

Most templates use `$SERVICE_..._X` style tokens (for example
`$SERVICE_PASSWORD_DB`, `$SERVICE_HEX_64_ENCRYPTIONKEY`) in place of literal
secrets in their Compose environment blocks. These are the same magic-var
convention a large share of real-world Compose template datasets use; there
is no resolver for them yet (see "Not built yet" below), so as of today they
deploy as literal, unresolved strings unless you edit them out first. The
dashboard's preview step shows the full Compose body specifically so you can
edit these (and image tags, ports, or anything else) before deploying.

## Static site detection (`build.type: static`)

Separately from the template catalog, an app whose `app.yaml` declares
`build.type: static` skips the container path entirely. `internal/deploy`'s
pipeline (`internal/deploy/static.go`) copies the built output directory
into `<staticRootDir>/<ServiceName>/<CommitSHA>` on the control plane host,
then saves that directory plus the service's domains as a `store.StaticSite`
row (`internal/store/static_site.go`, `migrations/0015_static_sites.sql`).
There is no image, no `store.DesiredService` row, and nothing for the
application controller to converge to: `internal/reconcile/ingress` reads
`store.StaticSite` rows directly and points Caddy's `file_server` handler at
each one's root directory. `spec.BuildStatic` services are also validated at
parse time to reject `port` and `host_port` in `app.yaml`, since there is no
running container to route a port to (`internal/spec/validate.go`).

Static sites currently have exactly one read surface: `GET
/api/v1/static-sites`, returning each site's `name` and `domains` (never its
`RootDir`, a control-plane-local filesystem path with no reason to leave the
server). There is no static-site creation endpoint: the only way to produce
one is a git push to an app whose spec declares `build.type: static`. The
dashboard's Apps page shows a read-only "Static sites" card
(`web/src/components/StaticSitesCard.tsx`) that renders nothing at all if no
static sites exist yet, so an operator who has never used this build type
never sees an empty card. There is no detail page and no delete action,
because neither exists on the backend yet.

## Browsing from the dashboard

The Apps page and the Databases page both expose a "New resource" wizard
(`CreateResourceWizard.tsx`) with a "Browse templates" card alongside "Docker
image," "Deploy from git," and "Docker Compose." Picking it opens
`BrowseTemplatesFields.tsx`: a searchable, category-grouped grid backed by
`GET /api/v1/service-templates` (search matches name, slogan, or category,
client-side, over the already-fetched list). Selecting a card fetches that
one entry's full body via `GET /api/v1/service-templates/{id}` and pre-fills
a name field (defaulting to the template's ID) plus an editable Compose
textarea, the exact same form `CreateComposeFields` uses for a hand-pasted
Compose file. Submitting calls the same `useDeployCompose()` mutation, which
is the same `POST /api/v1/apps/{name}/compose` request either path makes.
There is no template-specific deploy result screen either: success shows one
row per deployed service with a link to that service's own app page.

## End-to-end integration walkthrough

1. **Browse the catalog**:

   ```bash
   levelrail-cli templates list
   ```

   ```
   ID              NAME              CATEGORY         SLOGAN
   n8n             n8n               Automation       Build automations and connect your tools with a visual, node-based workflow editor.
   uptime-kuma     Uptime Kuma       Monitoring       A self-hosted uptime monitor with a clean dashboard for HTTP, TCP, DNS, and ping checks.
   minio           MinIO             Storage          S3-compatible object storage you run yourself, with a built-in web console.
   ```

2. **Inspect one entry, including its Compose body**:

   ```bash
   levelrail-cli templates get uptime-kuma
   ```

   ```
   id:                uptime-kuma
   name:              Uptime Kuma
   category:          Monitoring
   slogan:            A self-hosted uptime monitor with a clean dashboard for HTTP, TCP, DNS, and ping checks.
   documentation_url: https://github.com/louislam/uptime-kuma/wiki
   compose:
   services:
     uptime-kuma:
       image: louislam/uptime-kuma:1.23.13
       ports: ["3001:3001"]
       volumes:
         - uptime_kuma_data:/app/data
   ```

3. **Deploy it as an app**, either straight from the catalog or by hand:

   ```bash
   levelrail-cli templates deploy uptime-kuma --name my-status-page
   ```

   This is a thin wrapper: it fetches the entry's Compose body via `GET
   /api/v1/service-templates/{id}`, then makes the identical call `apps
   deploy-compose` makes. The two are equivalent:

   ```bash
   levelrail-cli templates get uptime-kuma --output json | jq -r .compose > uptime-kuma.compose.yaml
   levelrail-cli apps deploy-compose my-status-page --file uptime-kuma.compose.yaml
   ```

   Either way, the response is the same shape:

   ```json
   {
     "app_id": "my-status-page",
     "services": [
       { "name": "uptime-kuma", "image": "louislam/uptime-kuma:1.23.13" }
     ]
   }
   ```

4. **From the dashboard**: open the Apps or Databases page, click "New app"
   (or "New database"), pick "Browse templates," search or scroll to a
   card, review the pre-filled Compose body (edit it if a `$SERVICE_..._X`
   token needs replacing with a real value first), name the app, and click
   Deploy. The result panel links straight to each deployed service's app
   page.

5. **Check static site visibility** (a separate, unrelated read path): push
   an app whose `app.yaml` sets `build.type: static`, then confirm it shows
   up without a container:

   ```bash
   levelrail-cli static-sites list
   ```

   ```
   NAME              DOMAINS
   docs-site         docs.example.com
   ```

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/service-templates` | `read` |
| `GET` | `/api/v1/service-templates/{id}` | `read` |
| `POST` | `/api/v1/apps/{name}/compose` | `deploy` (plus `root` if the Compose body bind-mounts a host directory) |
| `GET` | `/api/v1/static-sites` | `read` |

The catalog routes are read-only and serve `internal/catalog.Templates`
directly, no store or database involved. The deploy route is shared with
`apps deploy-compose`, not a template-specific endpoint: a template deploy
and a hand-written Compose deploy are indistinguishable to the API once the
request body leaves the client.

## CLI

```bash
levelrail-cli templates list [flags]
levelrail-cli templates get <id> [flags]
levelrail-cli templates deploy <id> [--name NAME] [flags]

levelrail-cli static-sites list [flags]
```

`templates deploy` defaults the app name to the template's own `id`; pass
`--name` to deploy the same template again under a different name.
`static-sites` has one verb (`list`) because it has one backing route: it
exists as a quick filter over apps that are plain static sites, since `apps
list` already shows every app including these.

## Not built yet (deliberate follow-ups)

- **No magic-var resolver.** A real survey of a public template dataset
  found most entries lean on a competing platform's own auto-generated
  password/domain convention (`$SERVICE_PASSWORD_X`, `$SERVICE_FQDN_X`, and
  so on) rather than plain literal values. Levelrail's templates use the
  same tokens for authenticity to the format, but nothing here resolves them
  to real generated values yet: they deploy as literal strings unless edited
  first. This is real, separate follow-up work (ADR 015's own consequences
  section).
- **No catalog storage or admin UI.** The catalog is a hardcoded Go slice
  shipped with the binary. Adding, editing, or removing a template means
  shipping a new control plane build, not a database write or a dashboard
  action.
- **No third-party catalog import.** ADR 015 explicitly leaves open whether
  a full third-party dataset ever gets imported verbatim; today's 123-entry
  catalog is Levelrail's own curated set, not an import.
- **No static site creation or delete surface beyond git push.** `GET
  /api/v1/static-sites` is the only static-site route. There is no create
  endpoint (a static site only exists because an app's spec declared
  `build.type: static` and got deployed), no detail page, and no delete
  action.
- **No template change history or versioning.** A template's Compose body
  can change between control plane releases with nothing recording what an
  already-deployed app was created from.
