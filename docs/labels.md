---
title: Labels and namespaces
description: Group and filter apps, databases, stacks and nodes with free-form labels.
---

Labels are key and value pairs you attach to apps, databases, stacks and nodes. They do not change how anything runs. Use them to group and find things.

Keys are lowercase letters, digits, `.`, `_`, `-` or `/` (up to 63 characters). Values are letters, digits, `.`, `_` or `-` (up to 63 characters). The number of labels per resource is capped by the `APP_LABELS_MAX_PER_RESOURCE` environment variable (default 32).

## CLI

```bash
levelrail-cli label app web env=prod tier=frontend   # set
levelrail-cli label app web tier-                    # remove
levelrail-cli label db main                          # show
levelrail-cli label node node-1 region=eu --json
```

Filter lists with a selector. Terms are comma separated: `k=v`, `k!=v`, `k` (key exists), `!k` (key absent).

```bash
levelrail-cli get apps -l env=prod,tier!=db
levelrail-cli get nodes -l region=eu
```

## Namespaces

A namespace is the project a resource belongs to. The v2 resource view reports it as `metadata.namespace`, and `get`, `describe` and `delete` take `-n` or `--namespace` to scope to one project.

```bash
levelrail-cli get apps -n shop
levelrail-cli describe app web -n shop
```

## API

- `GET|PUT|PATCH /api/v1/{apps,databases,stacks}/{name}/labels`
- `PUT` replaces the set. An omitted `labels` field keeps the current set, `{"labels": {}}` clears it.
- `PATCH` takes `{"set": {...}, "remove": [...]}` and merges.
- List endpoints accept `?label=env=prod,tier!=db`. The v2 lists accept `labelSelector` and `namespace`.

## Dashboard

App and database overview pages show label chips with an Edit button. The apps and databases lists have a label filter box that takes the same selector.
