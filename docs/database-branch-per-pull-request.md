---
title: "Database branch per pull request"
sidebarTitle: "Database branch per PR"
description: "Create a database branch for every pull request with a GitHub Action, see the schema diff in one comment, and delete the branch when the pull request closes."
"og:url": "https://docs.levelrail.com/database-branch-per-pull-request"
---

Each pull request gets its own copy of a database, named `<database>-pr-<number>`. The action posts one comment with the branch state and a schema diff, updates it on every push, and deletes the branch when the pull request closes or merges.

## Workflow

```yaml
name: database-branch
on:
  pull_request:
    types: [opened, synchronize, reopened, closed]
permissions:
  contents: read
  pull-requests: write
concurrency:
  group: levelrail-db-${{ github.event.pull_request.number }}
  cancel-in-progress: false
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

- `LEVELRAIL_TOKEN`: use a dedicated token with the flat `read` ability and two policy statements. Allow `db:write` on `database:app-db` (creates schema branches and lets the CLI check the database) and allow `db:admin` on `database:app-db-pr-*` (reads, waits for and deletes only the pull request branches). `db:admin` on a branch also covers reading it. The token has no admin right on the database itself, so it cannot delete, reset or promote it, and it cannot touch the branches of other databases.
- Replace `app-db` with your database name. The prefix pattern `database:app-db-pr-*` works because policies accept a trailing `*`.
- `pull-requests: write` is needed only for the comment. Set `comment: "false"` to skip it and drop the permission. Set `comment-author` when `github-token` is an App or personal token, so the comment is found by its real author (default `github-actions[bot]`).

Use the `pull_request` event only. Never use `pull_request_target`: it runs with secrets for code you have not reviewed, and the action refuses to run on it. Pull requests from forks get no secrets from GitHub, so the action skips them with a notice and a green check.

The workflow file is read from the pull request branch. Anyone who can push a branch to the repository can change it and read the token, so treat the token as shared with every committer and keep it minimal. Schema mode is the default for that reason. A full copy needs `db:admin` on the database itself, which this token does not have, so run full copies with a separate token and only for authors you trust.

## Modes and expiry

| Input | Values | Default |
| --- | --- | --- |
| `mode` | `schema` copies tables only, `full` copies rows too | `schema` |
| `ttl` | `7d`, `12h`, `never` | `7d` |

The branch is not protected by default. Protection blocks delete and expiry, so a protected branch would never be cleaned up by the close step. `pr-close` refuses a protected branch unless you pass `--force`. The ttl is a safety net. A branch that nobody closed expires on its own.

## How it works

On `opened`, `synchronize` and `reopened` the action runs `levelrail-cli databases branch pr <database> --pr <number>`. The command creates the branch or reuses it if it exists, then waits until it is ready (five minutes by default). The same push twice is safe.

It then runs `databases branch diff` against the parent and renders the counts and the first 20 changes in a collapsed block. The migration SQL is never put in the comment. The comment carries a hidden marker, so later runs edit it in place.

On `closed` the action runs `databases branch pr-close <database> --pr <number> --yes` and edits the comment to "Branch deleted". A branch that is already gone counts as success.

The comment shows host, port and name only. Credentials are never printed by the action. To connect from your machine:

```bash
levelrail-cli databases connection-string app-db-pr-42 --reveal
```

`--reveal` is limited to owners, asks for your password and is audited. The same commands work outside GitHub:

```bash
levelrail-cli databases branch pr app-db --pr 42 --mode schema --ttl 7d
levelrail-cli databases branch pr-close app-db --pr 42 --yes
```

## Limits

Branch names include the database, so two databases can use the same pull request number. Names over 63 characters shorten the database part and add a six character hash of the full name, so they stay unique and repeatable.
