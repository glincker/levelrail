---
description: Deploy ready-made services from the built-in template catalog, save your own apps as templates, and see how templates, Compose and static sites relate.
---

# Service template catalog

A template is a curated Docker Compose file for a well-known service (n8n, Uptime Kuma, a Postgres-backed app, a local LLM runtime). Deploying one fills in a Compose body for you and sends it through the same endpoint as `levelrail-cli apps deploy-compose`. There is no separate template deploy path, so a template-deployed app behaves like any other Compose app.

This page covers how to deploy templates, how `$SERVICE_...` variables work, and how to save your own apps as templates. To see what is in the catalog, read the [template catalog](/template-catalog). The seven multi-service wiring examples are described in [Starter kit templates](/templates).

## Deploy a template

<Tabs :items="['Dashboard', 'CLI', 'API']">
<Tab value="Dashboard">

Open **Apps**, choose **New app**, then **Browse templates**. Search by name, slogan or category, or pick a category. Selecting a card pre-fills the app name (the template ID) and an editable Compose body. Review it, replace any `$SERVICE_...` token you want to set yourself, and deploy. The result lists each deployed service with a link to its app page.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli templates list
levelrail-cli templates get uptime-kuma
levelrail-cli templates deploy uptime-kuma --name my-status-page
```

`templates deploy` fetches the template's Compose body and makes the same call as `apps deploy-compose`. The name defaults to the template ID, so pass `--name` to deploy the same template twice. This is equivalent:

```bash
levelrail-cli templates get uptime-kuma --output json | jq -r .compose > uptime-kuma.compose.yaml
levelrail-cli apps deploy-compose my-status-page --file uptime-kuma.compose.yaml
```

</Tab>
<Tab value="API">

```bash
curl -H "Authorization: Bearer $TOKEN" https://levelrail.example.com/api/v1/service-templates
curl -X POST -H "Authorization: Bearer $TOKEN" https://levelrail.example.com/api/v1/service-templates/uptime-kuma/deploy
```

The list omits Compose bodies to stay small; `GET /api/v1/service-templates/{id}` returns one entry in full. The one-click `deploy` route refuses a template that still needs configuration (see below) with a `409`.

</Tab>
</Tabs>

A Compose document becomes one app with one service per Compose service, each reconciled independently.

## How templates are defined

The built-in catalog is compiled into the control plane binary and served from memory: there is no catalog table and no admin screen. Adding or changing a built-in template means shipping a new control plane build. Each entry has an ID, name, slogan, category, documentation link, a Compose body, and two optional hints:

- **Recommended memory**: a static advisory shown as a badge (for example "~6 GiB RAM recommended"). It is a rule of thumb. It is not checked against any node's free memory.
- **Requires GPU**: the entry reserves an NVIDIA GPU through `deploy.resources.reservations.devices`. It needs a node with a GPU and the NVIDIA container runtime; see [AI models](ai-models.md#gpu-nodes).

Only the `resources.reservations.devices` and `replicas` keys of a Compose `deploy:` block are read. Other Swarm-only keys are reported rather than silently ignored.

### Magic variables

Templates use `$SERVICE_<KIND>_<NAME>` tokens in place of literal secrets. At deploy time:

- `PASSWORD`, `USER`, `BASE64`, `HEX` and `REALBASE64` tokens are generated and stored as secrets. Every reference to the same name gets the same value, so an app and its database agree on `$SERVICE_PASSWORD_DB`. A generated value is never written into the stored Compose body.
- `FQDN` tokens resolve to `https://` plus the service's assigned domain, when the Compose file assigns one.
- Any token written with a default (`${SERVICE_FOO:-value}`) uses that default.
- Anything else has no value and is reported as unresolved. A template with an unresolved token is marked `requires_configuration`, and the one-click route refuses it. Deploy it through the dashboard's Compose editor, or with `apps deploy-compose`, after filling the value in.

## Save an app as a template

Custom templates turn an app you already run into a reusable one-click template. They are stored in the database, not in the binary.

<Steps>
<Step title="Save the app">

On the app's detail page, open the overflow menu and choose **Save as template**, or run:

```bash
levelrail-cli apps save-as-template my-app --description "staging-ready stack"
```

Levelrail derives a Compose body from the app's current desired state (one service, or every service of a multi-service app) and saves it with an ID of the form `custom-` plus 16 hex characters.

</Step>
<Step title="Find it">

In the dashboard it appears under **Your templates** in the same template grid. From the CLI, `levelrail-cli templates list --custom`.

</Step>
<Step title="Deploy or delete it">

A custom ID works anywhere a catalog ID does:

```bash
levelrail-cli templates deploy custom-a1b2c3d4e5f6a7b8 --name my-app-copy
levelrail-cli templates delete custom-a1b2c3d4e5f6a7b8
```

Deleting removes only the saved template. The source app and apps already deployed from it are untouched.

</Step>
</Steps>

A custom template never contains a secret value. Secret, database-derived and Vault env entries carry only a key name or reference, and each one becomes a `${SERVICE_SECRET_<key>}` placeholder. `SECRET` is not a generated kind, so such a template always has `requires_configuration` set and must be deployed through the Compose editor, where you supply the real values.

The saved Compose body does not capture everything about the app:

- **Domains** stay with the source app. A new app starts with none, as with `apps clone`.
- **Host port, bind address and bind mounts** are specific to the source machine.
- **Resources and health checks** have no Compose representation Levelrail can read back, so they are dropped.

## Static sites

An app whose `app.yaml` sets `build.type: static` has no container. The control plane copies the built output to a directory on its own host and Caddy serves it directly. These sites are created only by a git push to such an app. `levelrail-cli static-sites list` (and `GET /api/v1/static-sites`) lists their names and domains, and the Apps page shows a read-only **Static sites** card when any exist. See [app.yaml reference](app-spec-reference.md#build) for the build types.

## API and CLI reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/service-templates` | `read` |
| `GET` | `/api/v1/service-templates/{id}` | `read` (also resolves a custom ID) |
| `POST` | `/api/v1/service-templates/{id}/deploy` | `deploy` (custom IDs only when no configuration is needed) |
| `POST` | `/api/v1/apps/{name}/compose` | `deploy` (plus `root` if the Compose body bind-mounts a host directory) |
| `POST` | `/api/v1/apps/{name}/save-as-template` | `write`, scoped to `{name}` |
| `GET` | `/api/v1/templates/custom` | `read` |
| `DELETE` | `/api/v1/templates/custom/{id}` | `write` |
| `GET` | `/api/v1/static-sites` | `read` |

```bash
levelrail-cli templates list [--custom]
levelrail-cli templates get <id>
levelrail-cli templates deploy <id> [--name NAME]
levelrail-cli templates delete <id>
levelrail-cli apps save-as-template <name> [--template-name NAME] [--description TEXT]
levelrail-cli static-sites list
```

`templates delete` only removes custom templates; the built-in catalog is read-only.

## Limits

- There is no versioning. A template's Compose body can change between control plane releases, and nothing records what an already-deployed app was created from.
- Importing a catalog from an external source is not supported: the built-in set is the one in the [template catalog](/template-catalog). The reasoning is in [ADR 015](../adr/015-service-template-catalog-reversal.md).

## Next steps

<CardGroup :cols="2">
<Card title="Template catalog" href="/template-catalog">

Every built-in template by category.

</Card>
<Card title="Starter kit templates" href="/templates">

Seven multi-service wiring patterns.

</Card>
<Card title="Deploying apps" href="/deploying-apps">

Roll out, roll back and operate what you deployed.

</Card>
<Card title="app.yaml reference" href="/app-spec-reference">

Build types and service settings.

</Card>
</CardGroup>
