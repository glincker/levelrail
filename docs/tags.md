---
description: Label apps with tags such as team:payments, then filter the dashboard, CLI and API by them.
---

# Tags

Tags are labels you attach to apps to organize them by team, component, lifecycle stage, or anything else. An app can carry several tags. Tags are organizational only: they do not change how an app runs, restrict access, or gate a deploy.

## Rules

- A tag name is lowercase and is either a `key` or a `key:value` pair, such as `backend`, `team:payments` or `tier:edge`.
- The key is up to 32 characters (letters, digits, `.`, `_`, `-`) and the value up to 64 (the same set plus `/`). Both must start with a letter or digit. Anything else is rejected with a 400.
- Tag names are unique across the control plane.
- An app can carry at most 20 tags. Set `APP_MAX_TAGS_PER_APP` to change that. Tags created before these rules existed keep working.
- Tags apply to apps only, not databases.

## Create and delete tags

<Tabs :items="['CLI', 'API']">
<Tab value="CLI">

```bash
levelrail-cli tags list
levelrail-cli tags create --name team:payments
levelrail-cli tags apps team:payments       # apps carrying this tag
levelrail-cli tags delete team:payments     # detaches it from every app first
```

</Tab>
<Tab value="API">

```bash
curl -X POST https://control-plane/api/v1/tags \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"team:payments"}'

curl https://control-plane/api/v1/tags -H "Authorization: Bearer $TOKEN"
curl https://control-plane/api/v1/tags/TAG_ID/apps -H "Authorization: Bearer $TOKEN"
curl -X DELETE https://control-plane/api/v1/tags/TAG_ID -H "Authorization: Bearer $TOKEN"
```

</Tab>
</Tabs>

Deleting a tag removes it from every app it was on.

## Attach and detach

Attaching by name creates the tag if it does not exist yet, so you rarely need `tags create` first.

<Tabs :items="['Dashboard', 'CLI', 'API']">
<Tab value="Dashboard">

Use the tag control on an app's page to add a tag or remove one with the chip's close button.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli apps tag my-app team:payments
levelrail-cli apps untag my-app team:payments
```

Both take the tag by name.

</Tab>
<Tab value="API">

```bash
curl https://control-plane/api/v1/apps/my-app/tags -H "Authorization: Bearer $TOKEN"

curl -X POST https://control-plane/api/v1/apps/my-app/tags \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"team:payments"}'

curl -X DELETE https://control-plane/api/v1/apps/my-app/tags/TAG_ID -H "Authorization: Bearer $TOKEN"
```

The detach route takes the tag ID.

</Tab>
</Tabs>

If two requests race to create the same tag name, the one that loses attaches the tag the other created.

## Filter by tag

In the dashboard, every row on the Apps page (`/apps`) shows its tag chips. Click a chip to filter the list to apps with that tag; the filter bar shows the active filter and a clear button.

`GET /api/v1/apps` accepts these query parameters:

| Parameter | Meaning |
| --- | --- |
| `tag` | Repeat it or comma-separate values. An app must carry all of them. |
| `environment` | Environment name or ID |
| `project` | Project ID |
| `q` | Substring of the app name or image |
| `limit`, `offset` | Paging. Without `limit` the whole filtered set returns. |

The `X-Total-Count` response header carries the match count before paging. `GET /api/v1/apps-summary` takes the same filters and returns running, deploying, failing and stopped counts. Tags also select targets for `levelrail-cli apps bulk`. See [Managing apps at scale](./managing-apps-at-scale.md).

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/tags` | `write` |
| `GET` | `/api/v1/tags` | `read` |
| `DELETE` | `/api/v1/tags/{id}` | `write` |
| `GET` | `/api/v1/tags/{id}/apps` | `read` |
| `GET` | `/api/v1/apps/{name}/tags` | `read` |
| `POST` | `/api/v1/apps/{name}/tags` | `write` |
| `DELETE` | `/api/v1/apps/{name}/tags/{id}` | `write` |

## Next steps

<CardGroup :cols="2">
<Card title="Managing apps at scale" href="/managing-apps-at-scale">

Filters, bulk actions and summaries across many apps.

</Card>
<Card title="Projects and organizations" href="/projects-and-organizations">

A structural alternative to tags, with shared environment variables.

</Card>
</CardGroup>
