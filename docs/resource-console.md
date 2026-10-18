---
title: "Resource console"
description: Search every app, database, node and domain in one list, tag them in bulk with a preview, save views, and see cost grouped by tag.
---

The Resources page (`/resources`) lists every app, database, node and domain you can see in one searchable, virtualized list. Use it to find things across kinds, label them, and answer "what does the payments team cost".

## Search and filter

- Type in the search box to match a name, detail (image, engine, address) or tag.
- Pick a kind to narrow the list. Each kind button shows how many resources match the rest of your filters.
- The Tags control keeps only resources that carry all selected tags. Click a tag on a row to add it to the filter.
- Press `Cmd+K` (`Ctrl+K`) and type a tag name to jump to the list filtered by it. Saved views also appear in the palette.

Team scoped users only see resources they can read. Nodes and standalone domains are instance wide, so they only show for instance members.

## Tag many resources at once

Select rows, then choose Add tag or Remove tag. The dialog first runs a preview (a server side dry run) that shows how many resources would change, how many already match, and which ones are blocked by permissions. Nothing is written until you apply. Applying is only enabled once a preview for the same tag has passed.

Tags are the same ones used on apps (`team:payments`, `env:prod`), and now also work on databases, nodes and domains. The same limits apply: a tag is a lowercase `key` or `key:value`.

## Saved views

Saved views store a search, kind and tag filter under a name. They belong to you, and they show up in the Resources page and the command palette.

## Cost by tag

The Resources page shows the month to date cost for your top tags, and the Usage page has a Tag breakdown. Cost comes from the same allocation as the [usage report](/usage-and-cost), so prices must be configured. Apps and databases carry cost. A resource with several tags is counted under each of them, so tag totals can add up to more than the report total. Resources with no tag appear as Untagged.

## Command line

```bash
levelrail-cli resources list --kind app --tag team:payments
levelrail-cli resources tag --tag team:payments --dry-run app:web app:worker database:main
levelrail-cli resources tag --tag team:payments app:web app:worker database:main
levelrail-cli resources untag --tag team:payments app:worker
levelrail-cli resources views save payments --tag team:payments
levelrail-cli usage report --by tag
```

Resources are named `kind:name` where kind is `app`, `database`, `node` or `domain`. `--dry-run` writes nothing.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/resources` (`q`, `kind`, repeated `tag`, `limit`, `offset`) | `read` |
| `POST` | `/api/v1/resources/tags/bulk` (`action`, `tag`, `resources`, `dry_run`) | `write` |
| `GET` `POST` | `/api/v1/resources/views` | `read` |
| `DELETE` | `/api/v1/resources/views/{id}` | `read` |
| `GET` | `/api/v1/usage/report` returns a `tags` ranking | `read` |

The bulk call answers 207 with one result per resource: `ok`, `would_apply`, `skipped`, `denied`, `not_found` or `error`. Each resource is authorized on its own, so a denied one never fails the batch. A call may name up to 500 resources (`APP_BULK_MAX_RESOURCES`). The list returns at most 5000 rows (`APP_RESOURCE_EXPLORER_MAX`).

## Limits

- Buckets and networks are not listed yet. They join the explorer when those resources ship.
- Domain and node tags follow the name. Renaming a node leaves its old tags behind.
