---
description: Deploy ready-made services from a curated catalog using Docker Compose with one-step configuration
---

# Service template catalog

Deploy a curated set of ready-made services (n8n, Uptime Kuma, Postgres-backed apps, etc.) in one step, without hand-writing a Compose file.

::: details For contributors: where this lives in the source
- `internal/catalog/catalog.go` - template definitions and catalog assembly
- `internal/api/service_templates.go` - API handlers
- `cmd/levelrail-cli/templates.go` - CLI commands
:::

## Why this exists

The original non-goals list ruled out large template catalogs: ten good templates beat 280 stale ones. ADR 015 reversed this after comparing this platform's "New resource" modal (five plain cards) against a competing platform's categorized, searchable catalog. The founder asked for the real thing.

The constraint: every template in a dataset like that is a Docker Compose file. The reconciler (`internal/reconcile/application`) has only ever managed one container per service. So the sequencing was Compose support first, template catalog second, both landing together.

A template is not a separate deploy mechanism bolted on. It is a curated `compose.yaml` body handed to the exact same endpoint `apps deploy-compose` already uses. No template-specific reconciler path, no template-specific database table beyond static catalog data.

## How it actually works

`internal/catalog/` holds `Templates`, assembled in `catalog.go` from per-category Go slices (`templates_*.go`) of `Template` structs (`ID`, `Name`, `Slogan`, `Category`, `DocumentationURL`, `Compose`, `RecommendedMemoryBytes`, `RequiresGPU`). It is written fresh for this platform, not copied from other projects, and served straight out of memory. No database table, no admin UI, no version field.

Adding a template means adding an entry to that Go slice and shipping a new control plane binary.

`RecommendedMemoryBytes` is a static, pre-deploy advisory shown as a badge in the wizard (e.g. "~6 GiB RAM recommended"). It is not checked against any node's real available memory: no host memory monitoring exists anywhere in this codebase today (see [Architecture](architecture.md)), so treat it as a rule-of-thumb hint, not a live capacity check.

`RequiresGPU` marks entries (vLLM, Text Generation Inference, ComfyUI, Stable Diffusion WebUI, Jupyter GPU) that are only practical with an NVIDIA GPU. Their Compose bodies ship CPU defaults because `internal/compose` does not translate `deploy.resources.reservations.devices`, so GPU passthrough is not available yet.

::: details Local AI model templates (Ollama)
Six `Category: "AI"` entries run models locally through [Ollama](https://ollama.com): a bare `ollama` runtime plus five pre-configured to auto-pull one specific model on first start (Mistral 7B, Llama 3 8B, Qwen 2.5 7B, Phi-3 Mini, DeepSeek-R1 7B). Each pins an explicit parameter-size tag (e.g. `mistral:7b-instruct-v0.3`, not the bare `mistral` alias), since Ollama's own library docs note a bare alias's default can change when the publisher updates it. `RecommendedMemoryBytes` on these is computed from Q4 quantization's well-known rule of thumb (roughly 0.6 GB per billion parameters, plus ~1.5 GB of runtime overhead), not guessed. CPU-only: no GPU passthrough exists in this codebase yet.
:::

**API**

- `GET /api/v1/service-templates` returns the catalog without `Compose` bodies, keeping browse load small.
- `GET /api/v1/service-templates/{id}` returns one entry including full Compose text.

**Deployment**

Deploying is a separate explicit step. The caller (dashboard or CLI) fetches an entry's Compose body, then POSTs it to `POST /api/v1/apps/{name}/compose`, the same route `apps deploy-compose` uses for hand-written files. A Compose document fans out into one `store.App` plus one `store.DesiredService` per compose service (`migrations/0039_apps.sql`), each reconciled independently.

**Magic variables**

Most templates use `$SERVICE_..._X` style tokens (e.g. `$SERVICE_PASSWORD_DB`, `$SERVICE_HEX_64_ENCRYPTIONKEY`) in place of literal secrets in Compose environment blocks. This convention matches real-world Compose template datasets. There is no resolver yet (see "Not built yet" below), so they deploy as literal strings unless you edit them first. The dashboard's preview step shows the full Compose body so you can edit these before deploying.

## Static site detection (`build.type: static`)

An app whose `app.yaml` declares `build.type: static` skips the container path entirely. The pipeline (`internal/deploy/static.go`) copies the built output directory into `<staticRootDir>/<ServiceName>/<CommitSHA>` on the control plane host, then saves that directory plus domains as a `store.StaticSite` row (`internal/store/static_site.go`, `migrations/0015_static_sites.sql`).

There is no image, no `store.DesiredService` row, and nothing for the application controller to converge to. `internal/reconcile/ingress` reads `store.StaticSite` rows directly and points Caddy's `file_server` handler at each root directory.

`spec.BuildStatic` services are validated at parse time to reject `port` and `host_port` in `app.yaml` (no running container to route to).

**Read surface**

`GET /api/v1/static-sites` returns each site's `name` and `domains` (never `RootDir`, a control-plane-local filesystem path). No static-site creation endpoint exists. The only way to create one is a git push to an app declaring `build.type: static`.

The dashboard's Apps page shows a read-only "Static sites" card (`web/src/components/StaticSitesCard.tsx`) that renders nothing if no static sites exist. Operators who never use this build type never see an empty card. There is no detail page and no delete action yet.

## Browsing from the dashboard

The Apps and Databases pages both expose a "New resource" wizard (`CreateResourceWizard.tsx`) with a "Browse templates" card alongside "Docker image," "Deploy from git," and "Docker Compose."

Picking it opens `BrowseTemplatesFields.tsx`: a searchable, category-grouped grid backed by `GET /api/v1/service-templates` (search matches name, slogan, or category, client-side). Selecting a card fetches that entry's full body via `GET /api/v1/service-templates/{id}` and pre-fills:
- A name field (defaulting to template ID)
- An editable Compose textarea (the same form `CreateComposeFields` uses)

Submitting calls the same `useDeployCompose()` mutation, which is the same `POST /api/v1/apps/{name}/compose` request either path makes. There is no template-specific result screen. Success shows one row per deployed service with a link to that service's app page.

The `Starter Kits` category holds seven templates that demonstrate multi-service wiring patterns (a web tier plus a database, a worker plus a queue, a reverse proxy fanning out to two backends) rather than deploying a single known project; see [Starter kit templates](/templates) for the full list and what each one demonstrates.

## Custom templates (save an app as a template)

The catalog above is static and ships with the binary. Custom templates are the operator-defined counterpart: turn an app you already have configured and running into a reusable, one-click template, stored in the database (`custom_templates` table, `migrations/0270_custom_templates.sql`) instead of compiled into the binary. This is the growth-loop version of the catalog: replicate your own stack, or hand a teammate a template of something you already run, without either of you hand-writing a Compose file.

::: details For contributors: where this lives in the source
- `internal/compose/export.go` - `FromDesiredServices`, the app-state-to-compose.yaml derivation
- `internal/store/custom_template.go` - the `custom_templates` table
- `internal/api/service_templates_custom.go` - API handlers
- `cmd/levelrail-cli/apps_save_as_template.go`, `cmd/levelrail-cli/templates.go` - CLI commands
:::

**How it works**

`POST /api/v1/apps/{name}/save-as-template` reads `{name}`'s current desired state (one service, or every member service of a multi-service app) and derives a `compose.yaml` body from it with `compose.FromDesiredServices`, the structural reverse of the same `ToDesiredServices` function a catalog deploy or a hand-written Compose deploy already goes through. The result is saved as a `store.CustomTemplate` row: an id (`custom-` plus 16 random hex characters, so it can never collide with a catalog slug), a name, an optional description, and the derived Compose body.

Because the output is an ordinary `compose.yaml` body in the same shape a catalog entry already has, a custom template id works through the exact same read and deploy routes the built-in catalog uses (`resolveTemplate` in `internal/api/service_templates.go` tries the static catalog first, then falls back to the custom template store): `GET /api/v1/service-templates/{id}` and `POST /api/v1/service-templates/{id}/deploy` both resolve a custom template id transparently. There is no second deploy mechanism.

**Secret handling (read this before relying on it)**

A custom template never contains a real secret value. This is enforced structurally, not by a filter that could miss a field:

- `store.DesiredService.SecretEnv`, `.DatabaseEnv`, and `.VaultEnv` never carry a decrypted value in the first place, only a key name or reference (`internal/secrets` resolves the real value separately, immediately before container creation, and always has). There is nothing for the export step to accidentally copy.
- Every one of those captured keys becomes a `${SERVICE_SECRET_<key>}` placeholder in the saved Compose body, deliberately never reusing the key's own name as the magic-var "kind" segment: a secret literally named e.g. `PASSWORD_RESET_SALT` would otherwise risk being misread as the auto-generatable `PASSWORD` kind by `internal/compose`'s own `SERVICE_<KIND>_<rest>` convention and silently filled with a random value. `SECRET` is not one of the generatable kinds (`PASSWORD`, `USER`, `BASE64`, `HEX`, `REALBASE64`), so it is always reported unresolved.
- An unresolved placeholder makes `requires_configuration: true` on the saved template, the same signal a built-in catalog entry with a required secret already has. The one-click deploy path (`POST /api/v1/service-templates/{id}/deploy`) refuses a template in that state with a 409; deploying it instead goes through the dashboard's pre-filled Compose textarea (or `templates get` on the CLI), where the real value is supplied before the request is sent.

Also dropped, each for its own reason (not a secret-handling concern, but worth knowing before relying on a saved template as a full backup of an app's configuration):

- **Domains** - the source app still owns them; a deployed-from-template app starts domainless, same as `apps clone`.
- **HostPort, BindAddress, and bind mounts** - host-specific placement and host filesystem paths from the source machine, meaningless (or outright wrong) once redeployed elsewhere.
- **Resources (memory/CPU/swap) and health checks** - `internal/compose`'s own `deploy:` block only reads GPU device reservations and `replicas:` (see "Not built yet" above); there is no Compose representation for the rest that this same package can parse back.

See `internal/compose/export.go`'s own doc comment on `FromDesiredServices` for the exact field-by-field list.

**Listing and deleting**

`GET /api/v1/templates/custom` lists every saved custom template (without its Compose body, same list/detail split as the built-in catalog). `DELETE /api/v1/templates/custom/{id}` removes one; this never touches the source app or any app already deployed from the template, it only removes the saved template row itself.

In the dashboard, custom templates show up under a "Your templates" category in the same template marketplace grid the built-in catalog uses (`web/src/components/TemplateMarketplace.tsx`), with their own delete action; "Save as template" is an action on the app detail page's overflow menu.

From the CLI:

```bash
levelrail-cli apps save-as-template my-app --description "staging-ready stack"
levelrail-cli templates list --custom
levelrail-cli templates get custom-a1b2c3d4e5f6a7b8
levelrail-cli templates deploy custom-a1b2c3d4e5f6a7b8 --name my-app-copy
levelrail-cli templates delete custom-a1b2c3d4e5f6a7b8
```

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

3. **Deploy it as an app**, either from catalog or by hand:

   ```bash
   levelrail-cli templates deploy uptime-kuma --name my-status-page
   ```

   This is a thin wrapper. It fetches the entry's Compose body via `GET /api/v1/service-templates/{id}`, then makes the identical `apps deploy-compose` call. The two are equivalent:

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
| `GET` | `/api/v1/service-templates/{id}` | `read` (also resolves a custom template id) |
| `POST` | `/api/v1/service-templates/{id}/deploy` | `deploy` (also deploys a custom template id, when it needs no configuration) |
| `POST` | `/api/v1/apps/{name}/compose` | `deploy` (plus `root` if the Compose body bind-mounts a host directory) |
| `POST` | `/api/v1/apps/{name}/save-as-template` | `write`, scoped to `{name}` |
| `GET` | `/api/v1/templates/custom` | `read` |
| `DELETE` | `/api/v1/templates/custom/{id}` | `write` |
| `GET` | `/api/v1/static-sites` | `read` |

The catalog routes are read-only and serve `internal/catalog.Templates`
directly, no store or database involved. The deploy route is shared with
`apps deploy-compose`, not a template-specific endpoint: a template deploy
and a hand-written Compose deploy are indistinguishable to the API once the
request body leaves the client. A custom template id flows through that
same `GET`/`POST .../deploy` pair, `resolveTemplate` falling back to the
`custom_templates` table when the id doesn't match the static catalog; see
"Custom templates" above.

## CLI

```bash
levelrail-cli templates list [--custom] [flags]
levelrail-cli templates get <id> [flags]
levelrail-cli templates deploy <id> [--name NAME] [flags]
levelrail-cli templates delete <id> [flags]

levelrail-cli apps save-as-template <name> [--template-name NAME] [--description TEXT] [flags]

levelrail-cli static-sites list [flags]
```

`templates deploy` defaults the app name to the template's own `id`; pass
`--name` to deploy the same template again under a different name. `templates
list --custom` lists your own saved templates instead of the built-in
catalog; `templates delete` only ever removes a custom template, the
built-in catalog is read-only. `static-sites` has one verb (`list`) because
it has one backing route: it exists as a quick filter over apps that are
plain static sites, since `apps list` already shows every app including
these.

::: details Not built yet (deliberate follow-ups)

**No magic-var resolver**

Competing platforms auto-generate values via convention (`$SERVICE_PASSWORD_X`, `$SERVICE_FQDN_X`). Levelrail templates use the same tokens for format authenticity, but nothing resolves them to generated values yet. They deploy as literal strings unless edited first. This is separate follow-up work (ADR 015's consequences section).

**No catalog storage or admin UI**

The catalog is a hardcoded Go slice shipped with the binary. Adding, editing, or removing a template requires shipping a new control plane build, not a database write or dashboard action.

**No third-party catalog import**

ADR 015 leaves open whether a full third-party dataset gets imported verbatim. Today's 206-entry catalog is Levelrail's own curated set, not an import.

**No static site creation or delete surface beyond git push**

`GET /api/v1/static-sites` is the only static-site route. A static site only exists because an app's spec declared `build.type: static` and deployed. There is no create endpoint, no detail page, and no delete action.

**No template change history or versioning**

A template's Compose body can change between control plane releases with nothing recording what an already-deployed app was created from.

:::

## See also

- [Deploying apps user guide](deploying-apps.md) - using templates from the dashboard or CLI
- [App spec reference](app-spec-reference.md) - YAML format and build type options
- [ADR 015: Service template catalog reversal](../adr/015-service-template-catalog-reversal.md) - design decision and rationale
