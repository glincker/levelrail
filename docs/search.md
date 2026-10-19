---
title: "Unified search"
tag: "Beta"
description: One search over apps, databases, nodes, domains, templates, settings and actions, filtered by what you are allowed to see.
---

Search finds apps, databases, nodes, domains, deployments, templates, projects, environments, tags, settings pages and actions in one box. It works from the API, the CLI and the MCP server, and the dashboard command palette reads the same endpoint.

## Query syntax

Scopes narrow the result, everything else is free text. Free text matches any part of a name, so `rapd` still finds `rapid-worker`.

| Scope | Meaning |
| --- | --- |
| `app:NAME` | Only things that belong to an app whose name contains NAME |
| `env:NAME` | Only things in an environment |
| `tag:NAME` | Only apps with a tag |
| `node:NAME` | Only things on a node |
| `kind:KIND` | One kind: app, database, node, domain, template, project, environment, tag, deployment, setting, doc, action |

Example: `env:prod kind:app api` lists production apps with `api` in the name.

## Actions

An action is something you can do to an app or database: deploy, restart, open logs, rollback, stop, add a domain, back up a database now. `search kind:action restart api` returns the Restart action for every app whose name matches `api` and that you may deploy, and nothing else. Each action hit carries the ability it needs and whether it asks to confirm. Search lists actions and never runs them.

## What you can see

Every hit is checked against your own access before it is ranked, with the same rules the rest of the API uses: IAM policies and denies, environment visibility, team scoping and the ability a route needs. A token that cannot see an app gets no hit for it, no action on it, and no domain, deployment or tag that would reveal it. Nodes appear only to callers who may list nodes. Members of a non-default team can search too: they see their own team's apps, databases, projects and environments, plus templates and help pages. Tags and settings pages are instance wide, so tenants never get those hits.

## Ranking

Text relevance first, weighted by kind (apps and databases lead), then recency, then how often you opened a hit. Usage is per person.

## Saved searches

Save a query under a name and find it again by searching for the name. Saved searches are private to you. Saving needs write access, so a read-only token can search but cannot save.

```
levelrail-cli search saved add prod-apis env:prod kind:app
levelrail-cli search saved list
```

## Command palette

`Cmd+K` (or `Ctrl+K`) opens the palette, which searches through this service instead of downloading lists. Opening it sends no search request; typing sends one, 100 ms after you pause.

- Results are grouped by kind in relevance order, four per group (six for actions) with a "Show all" row to expand.
- Type `app:web ` or `env:prod ` and the finished scope becomes a chip. Backspace on empty text removes the last chip, and typing `ap` or `app:we` offers completions.
- Actions are only the ones the API returned for you. Enter runs one, and destructive ones (deploy, restart, rollback, stop, back up) ask to confirm first. Tab or `Cmd+Enter` is the secondary action: logs for an app, a new tab for anything else.
- Recent searches and saved searches show when the box is empty. "Save search" stores the current query, and the cross on a saved row deletes it.
- If search is down the palette says so, offers Retry and keeps the built-in navigation commands working.
- The settings page search box uses `kind:setting` through the same endpoint, so it also finds settings by their search keywords.

## API

| Route | Purpose |
| --- | --- |
| `GET /api/v1/search?q=&kind=&limit=&cursor=` | Ranked, paged hits |
| `GET /api/v1/search/suggest?q=` | Scope completions and top titles |
| `GET /api/v1/search/saved` | Your saved searches |
| `POST /api/v1/search/saved` | Save one (`name`, `query`) |
| `DELETE /api/v1/search/saved/{id}` | Remove one |
| `POST /api/v1/search/usage` | Record that you opened a hit (`ref`) |

The index refreshes within a couple of seconds of any change made through the API and fully every minute, so a hit can lag a change made outside the API by up to a minute. Measured latency at 1000 apps is in [benchmarks](/benchmarks).

Each hit carries `title_spans`, the character ranges of the title that matched (counted in Unicode code points), so clients highlight exactly what the server matched, including typo matches.

`GET /api/v1/apps?q=` uses the same ranking: exact and prefix names first, typos tolerated, and apps the caller cannot read are never counted. Until the index has been built, or when nothing ranks, the list falls back to a plain substring match on name and image. `limit`, `offset` and `X-Total-Count` behave as before.

The MCP tools `search` and `search_actions` run the same search and only list what the token may see. An action hit tells an agent what to run: `mcp_tool` is the exact tool name, `mcp_args` the arguments already filled in (the target `name`), and `mcp_needs` the inputs it must still supply, for example `image` for `deploy_app`. An empty `mcp_tool` means no tool exists for that action, so use the hit's `href`.
