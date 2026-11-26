---
title: "Database branch per pull request"
sidebarTitle: "Database branch per PR"
description: "Create a database branch for every pull request with a GitHub Action, see the schema diff in one comment, and delete the branch when the pull request closes."
"og:url": "https://docs.levelrail.com/database-branch-per-pull-request"
---

Each pull request gets its own copy of a database, named `pr-<number>`. The action posts one comment with the branch state and a schema diff, updates it on every push, and deletes the branch when the pull request closes or merges.

## Workflow

```yaml
name: database-branch
on:
  pull_request:
    types: [opened, synchronize, reopened, closed]
permissions:
  contents: read
  pull-requests: write
jobs:
  branch:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: GLINCKER/levelrail/actions/database-branch@v1.5.7
        with:
          version: v1.5.7
          server: https://levelrail.example.com
          token: ${{ secrets.LEVELRAIL_TOKEN }}
          database: app-db
          mode: schema
          ttl: 7d
```

Outputs: `branch`, `host`, `port` and `database-name`. Use them in later steps to run migrations against the branch.

## Secrets and permissions

- `LEVELRAIL_TOKEN`: an API token that can create and delete database branches. Give it no more than that and rotate it on a schedule. Pull requests from forks do not receive repository secrets, so the action does nothing for them.
- `pull-requests: write`: needed only for the comment. Set `comment: "false"` to skip it and drop the permission.

## Modes and expiry

| Input | Values | Default |
| --- | --- | --- |
| `mode` | `schema` copies tables only, `full` copies rows too | `schema` |
| `ttl` | `7d`, `12h`, `never` | `7d` |

The ttl is a safety net. A branch that nobody closed expires on its own.

## How it works

On `opened`, `synchronize` and `reopened` the action runs `levelrail-cli databases branch pr <database> --pr <number>`. The command creates the branch or reuses it if it exists, then waits until it is ready (five minutes by default). The same push twice is safe.

It then runs `databases branch diff` against the parent and renders the counts and the first 20 changes in a collapsed block. The migration SQL is never put in the comment. The comment carries a hidden marker, so later runs edit it in place.

On `closed` the action runs `databases branch pr-close <database> --pr <number> --yes` and edits the comment to "Branch deleted". A branch that is already gone counts as success.

The comment shows host, port and name only. Credentials are never printed by the action. To connect from your machine:

```bash
levelrail-cli databases connection-string pr-42 --reveal
```

`--reveal` is limited to owners, asks for your password and is audited. The same commands work outside GitHub:

```bash
levelrail-cli databases branch pr app-db --pr 42 --mode schema --ttl 7d
levelrail-cli databases branch pr-close app-db --pr 42 --yes
```

## Limits

Branch names are `pr-<number>` and database names are unique on a server. Run one branching workflow per server, or the second repository reuses the first one's name and the command reports a conflict.
