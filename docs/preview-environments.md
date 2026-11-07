---
title: "Preview environments"
description: Run a copy of an app for each pull request, with its own URL, caps, optional database, and automatic teardown when the pull request closes.
"og:url": "https://docs.levelrail.com/preview-environments"
---

A preview environment is a running copy of an app for one pull request. Levelrail deploys it when the pull request opens or gets new commits, and tears it down when the pull request closes or merges.

Previews are off until you turn them on for an app. Each app opts in on its own.

## Turn it on

Connect the app to a git repository first (see [git integrations](/git-integrations)). Then:

```sh
levelrail-cli apps previews enable web
levelrail-cli apps previews list web
levelrail-cli apps previews disable web
```

`list` with no app name shows previews across every app you can read.

## Lifecycle

- **Opened or reopened** deploys a preview. A new push to the pull request redeploys it.
- **Closed or merged** tears it down, whether or not it merged.
- **Teardown** removes the preview app, its containers, its disposable databases, and its env overrides.
- **Stale sweep** tears down previews that have had no updates for their TTL. The default is 7 days. Set it per app with `--ttl-hours`, or for the whole control plane with `APP_PREVIEW_TTL` (a Go duration such as `168h`). The sweeper runs every hour by default, set by `APP_PREVIEW_SWEEP_INTERVAL`.
- A preview stuck in deploying longer than `APP_PREVIEW_STUCK_AFTER` (default 30 minutes) is marked failed by the startup sweep.

Manual controls:

```sh
levelrail-cli apps previews teardown web 42
levelrail-cli apps previews extend web 42 --hours 48
levelrail-cli apps previews sweep
```

`teardown` removes one pull request's preview now. `extend` keeps one preview past its TTL. `sweep` removes every stale preview across all apps.

## Limits

Two caps bound how many previews run at once:

- `APP_PREVIEW_ENV_MAX_PER_APP` (default 10) caps live previews per app.
- `APP_PREVIEW_ENV_MAX_TOTAL` (default 50) caps live previews on the control plane.

A value of 0 turns that cap off. An app can set its own per-app cap with `--max-previews`, where 0 means the platform cap applies.

When a cap is full, the app's `on_limit` decides what happens:

```sh
levelrail-cli apps previews limits web --on-limit evict_oldest   # default: remove the oldest live preview
levelrail-cli apps previews limits web --on-limit reject         # refuse the new preview
levelrail-cli apps previews limits                               # platform-wide usage and caps
```

A preview that the cap rejects is recorded with the status `limit_reached` and the reason.

## Fork pull requests

By default a pull request from a fork is held. It does not deploy until someone approves it. Approve a held pull request once with:

```sh
levelrail-cli apps previews approve web 42 --yes
```

Add `--share-secrets` to give that one deploy the app's environment variables and secrets. Without it, a fork preview gets no app environment.

To skip the hold for forks, set `--allow-forks`. Forks then deploy without approval but still get no app environment. Only `--fork-secrets` gives fork previews the app's environment and secrets, so turn it on only if you accept that a fork's code runs with them:

```sh
levelrail-cli apps previews settings web --allow-forks
levelrail-cli apps previews settings web --fork-secrets
```

## Database strategy

Each preview can get a database. The default is `none`, which gives the preview no database.

```sh
levelrail-cli apps previews settings web --database fresh
levelrail-cli apps previews settings web --database seed --seed-database web-db
```

| Strategy | What the preview gets |
| --- | --- |
| `none` | No database. This is the default. |
| `shared` | The app's production database. Writes from the preview land in production, so use this only when that is what you want. |
| `fresh` | An empty database of the same engine and version, created for this preview and removed with it. |
| `seed` | A fresh database seeded from the latest backup of `--seed-database`, or of the app's own database when no name is given. Seeding stops after `APP_PREVIEW_SEED_TIMEOUT` (default 30 minutes). |

The strategy applies to single-service apps. A multi-service app, or an app whose `app.yaml` declares `databases:`, uses that file instead. Mark a database with `ephemeralInPreviews: true` there to give every preview its own copy.

## Resources, idle sleep, and visibility

- Each preview gets 256Mi of memory and 0.25 CPU by default. Change them with `--memory` and `--cpu`.
- A preview sleeps after 30 minutes without requests. Change it with `--idle-sleep-minutes`, or for the control plane with `APP_PREVIEW_IDLE_SLEEP_MINUTES`.
- Search engines do not index preview URLs by default. Add `--allow-indexing` to allow it.
- Put basic auth in front of every preview of an app with `--gate`, plus `--gate-username` and `--gate-password`. The password is write-only: it is stored, not shown again.

```sh
levelrail-cli apps previews settings web --memory 512Mi --cpu 0.5 --idle-sleep-minutes 60
levelrail-cli apps previews settings web --gate --gate-username reviewer --gate-password "$PREVIEW_GATE_PASSWORD"
```

Run `levelrail-cli apps previews settings web` with no policy flag to see the current values.

## Pull request feedback

Previews can report back to GitHub: a commit status on each deploy, and one pull request comment that is updated in place. This is off by default and opts in per app:

```sh
levelrail-cli apps previews pr-status enable web
levelrail-cli apps previews pr-status disable web
```

The control plane can switch commit statuses and deployments off for every app with `APP_GIT_STATUS_ENABLED=false`.

## Environment overrides for one pull request

Set a value for one pull request only, for example to point one preview at a staging API. The override takes precedence over the app's environment for that pull request, and applies on its next deploy.

```sh
levelrail-cli apps pr-env set web FEATURE_FLAG --pr 42 --value on
levelrail-cli apps pr-env set web API_KEY --pr 42 --value sk-test-123 --secret
levelrail-cli apps pr-env list web --pr 42
levelrail-cli apps pr-env clear web <id> --pr 42
```

A secret is stored encrypted and its value is not returned by `list`. Use `--secret` for anything sensitive.

## Branch deploys

A branch deploy gives a branch a stable URL of its own. It is separate from pull request previews and rebuilds on every push to the branch.

```sh
levelrail-cli apps branch-deploys set web --mode patterns --branch staging --branch "feature/*"
levelrail-cli apps branch-deploys list web
levelrail-cli apps branch-deploys delete web staging
```

- `--mode` is `off`, `all`, or `patterns`. Only `all` and `patterns` give branches a deploy. In `patterns`, a pattern is an exact branch name or a glob where `*` matches any characters, including `/`. The production branch never gets one.
- A branch deploy is removed when the branch is deleted, or after 14 days with no push. `APP_BRANCH_DEPLOY_TTL` changes the idle time.
- `APP_BRANCH_DEPLOY_MAX_PER_APP` (default 10) caps live branch deploys per app. A push past the cap is ignored with a note.
- Branch-scoped env overrides are set with `levelrail-cli apps branch-env`.

## Not yet

- **Static site previews.** Static sites publish a preview under the name `<app>-pr-<number>`. This page does not yet cover their files, gate, or teardown in detail.
- **Pull request comment commands.** The control plane accepts commands in pull request comments. They are not documented here yet.
- **Preview databases for branch deploys.** The branch deploy code path does not set up a database. Treat a branch deploy as app-only until this is confirmed.
